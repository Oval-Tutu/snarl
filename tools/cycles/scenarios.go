package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/scenario"
)

// gridThrottles are the throttle rows of the grid.
var gridThrottles = []float64{0, 0.25, 0.5, 0.75, 1}

// closeOpenSeconds and closeSeconds are the full-throttle and the closed
// holds of each closing step.
const (
	closeOpenSeconds = 1.0
	closeSeconds     = 3.0
)

// gridOptions set the grid scenarios.
type gridOptions struct {
	step, settle, minHold float64
	cycles                int
	cutEvery              int
}

func runScenarios(args []string, stdout io.Writer) error {
	fs := newFlags("scenarios")
	holds := fs.String("holds", "", "a holds scenario of the engine (scenarios/upstream/<engine>/holds-t100.scn): its first and last hold give idle and the top rpm (required)")
	name := fs.String("name", "", "engine name for the file headers (required)")
	out := fs.String("out", "", "directory to write the grid scenarios to (required)")
	var o gridOptions
	fs.Float64Var(&o.step, "step", 250, "RPM step of the grid")
	fs.Float64Var(&o.settle, "settle", 0.5, "settle time in seconds at the start of each hold")
	fs.IntVar(&o.cycles, "cycles", 24, "whole engine cycles in each hold after the settle time")
	fs.Float64Var(&o.minHold, "min-hold", 1, "least hold time in seconds after the settle time")
	fs.IntVar(&o.cutEvery, "cut-every", 4, "the ignition-off holds take every Nth grid RPM and the top")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *holds == "" || *name == "" || *out == "" || fs.NArg() != 0 {
		fs.Usage()
		return errors.New("need --holds, --name and --out")
	}
	if o.step <= 0 || o.settle < 0 || o.cycles < 1 || o.cutEvery < 1 {
		return errors.New("--step, --cycles and --cut-every must be positive and --settle not negative")
	}
	sc, err := scenario.ParseFile(*holds)
	if err != nil {
		return err
	}
	var lo, hi float64
	for _, s := range sc.Segments {
		if s.Kind != scenario.Hold {
			continue
		}
		if lo == 0 {
			lo = s.RPM0
		}
		hi = s.RPM0
	}
	if lo == 0 || hi <= lo {
		return fmt.Errorf("%s: need at least two holds", *holds)
	}
	rpms := gridRPMs(lo, hi, o.step)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	files := gridFiles(*name, rpms, o)
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(*out, f.name), []byte(f.text), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "%s: %d grid rpm from %g to %g, %d files in %s\n", *name, len(rpms), lo, hi, len(files), *out)
	return nil
}

// gridRPMs returns idle, the multiples of step above it, and the top.
func gridRPMs(lo, hi, step float64) []float64 {
	out := []float64{lo}
	for r := math.Floor(lo/step)*step + step; r < hi-step/4; r += step {
		if r-lo >= step/4 {
			out = append(out, r)
		}
	}
	return append(out, hi)
}

// holdSeconds returns the length of a grid hold at rpm.
func holdSeconds(rpm float64, o gridOptions) float64 {
	s := o.settle + max(o.minHold, float64(o.cycles)*120/rpm)
	return math.Ceil(s*10) / 10
}

type gridFile struct {
	name, text string
}

// gridFiles returns one scenario per throttle row, and the ignition-off
// holds at throttle 0 and 1.
func gridFiles(name string, rpms []float64, o gridOptions) []gridFile {
	head := func(b *strings.Builder, what string) {
		fmt.Fprintf(b, "# %s: written by tools/cycles scenarios. Cycle table grid: %s.\n", name, what)
		fmt.Fprintf(b, "# Each hold is %g s of settling and at least %d whole cycles; 0.25 s sweeps join the holds.\n", o.settle, o.cycles)
		b.WriteString("# The renderer reads cut RPM THROTTLE SECONDS as a hold with the ignition off. snarl does not read these files.\n")
	}
	var files []gridFile
	for _, th := range gridThrottles {
		var b strings.Builder
		head(&b, fmt.Sprintf("holds at throttle %g", th))
		for i, r := range rpms {
			if i > 0 {
				fmt.Fprintf(&b, "sweep %g %g %g 0.25\n", rpms[i-1], r, th)
			}
			fmt.Fprintf(&b, "hold %g %g %g\n", r, th, holdSeconds(r, o))
		}
		files = append(files, gridFile{fmt.Sprintf("t%03d.scn", int(math.Round(th*100))), b.String()})
	}
	var cut []float64
	for i, r := range rpms {
		if i%o.cutEvery == 0 || i == len(rpms)-1 {
			cut = append(cut, r)
		}
	}
	var b strings.Builder
	head(&b, "ignition-off holds at throttle 0 and 1")
	prev := 0.0
	for _, th := range []float64{0, 1} {
		for _, r := range cut {
			if prev > 0 {
				fmt.Fprintf(&b, "sweep %g %g %g 0.25\n", prev, r, th)
			}
			fmt.Fprintf(&b, "hold %g %g 0.5\n", r, th)
			fmt.Fprintf(&b, "cut %g %g %g\n", r, th, holdSeconds(r, o))
			prev = r
		}
	}
	files = append(files, gridFile{"cut.scn", b.String()})

	// The closing steps: full throttle, then closed for closeSeconds, at the
	// RPM of the ignition-off holds, for the closing transient.
	b.Reset()
	fmt.Fprintf(&b, "# %s: written by tools/cycles scenarios. Closing steps for the closing transient of the cycle tables:\n", name)
	fmt.Fprintf(&b, "# full throttle for %g s, then closed for %g s, at each RPM. snarl does not read this file.\n", closeOpenSeconds, closeSeconds)
	prev = 0
	for _, r := range cut {
		if prev > 0 {
			fmt.Fprintf(&b, "sweep %g %g 1 0.25\n", prev, r)
		}
		fmt.Fprintf(&b, "hold %g 1 %g\nhold %g 0 %g\n", r, closeOpenSeconds, r, closeSeconds)
		prev = r
	}
	files = append(files, gridFile{"close.scn", b.String()})

	// The throttle step: full throttle, closed, full again, at the grid
	// RPM nearest 60 per cent of the top.
	step := rpms[0]
	for _, r := range rpms {
		if math.Abs(r-0.6*rpms[len(rpms)-1]) < math.Abs(step-0.6*rpms[len(rpms)-1]) {
			step = r
		}
	}
	b.Reset()
	fmt.Fprintf(&b, "# %s: written by tools/cycles scenarios. Throttle steps at %g rpm for the intake lag of the cycle tables:\n", name, step)
	b.WriteString("# full throttle, closed, then full again. snarl does not read this file.\n")
	fmt.Fprintf(&b, "hold %g 1 1.5\nhold %g 0 3\nhold %g 1 3\n", step, step, step)
	files = append(files, gridFile{"step.scn", b.String()})
	return files
}
