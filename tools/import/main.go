// Command import converts the engine profiles of engine-sim and EngineLab
// to snarl profiles for the test catalogue in profiles/catalogue. It reads
// the JSON dumps that just import-profiles makes, fits the formant bank of
// each engine to its engine-sim impulse response and the pop bank to its
// earlier EngineLab pop stages, writes one TOML profile and one scenario
// directory for each engine, and writes profiles/catalogue/README.md. The
// catalogue is in the format before the cycle tables, which snarl does not
// load, so its output level is not calibrated.
//
// With --reference, it writes the reference set of just import-reference
// in place of the catalogue: five engine-sim engines from their upstream
// files, their cycle tables and their own upstream reference renders, each
// with a sources file that gives the source of every key, the impulse
// responses that the tables name, and the README of the set
// (reference.go). --scenarios then holds the upstream scenarios
// (scenarios/upstream).
//
// Usage:
//
//	go run ./tools/import --engine-sim DIR --enginelab DIR --profiles DIR --scenarios DIR [--sound-library DIR]
//	go run ./tools/import --reference profiles --scenarios scenarios/upstream [--grid DIR] [--engine-sim-share DIR] [--ref id=DIR ...]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// engine is an imported engine in the units of a snarl profile.
type engine struct {
	ID          string
	Project     string // "engine-sim" or "EngineLab"
	File        string // upstream file, relative to the engine directory
	Description string
	Layout      string

	IdleRPM, RedlineRPM, LimiterRPM, LimiterMS float64
	LimiterKind                                string
	Crossover                                  string
	Coupling                                   float64
	Cylinders                                  []cylinder
	Paths                                      []exhaustPath
	// Notes are the conversion decisions for this engine, for the README.
	Notes []string

	// The engine-sim synthesiser settings: hf_gain, noise and jitter, and
	// the impulse response file that the formant bank follows.
	HFGain, AM, InputJitter float64
	ImpulseResponse         string

	// Formant and pop banks, and the pop delay in milliseconds.
	Bank, PopBank analysis.FormantBank
	PopDelayMS    float64

	// PulseTable is the pulse table of the reference engine closest in
	// cylinder count.
	PulseTable string

	// Calibration results.
	LevellerDB, OutputDB, SweepLUFS float64
}

type cylinder struct {
	CrankDeg float64
	Bank     int
	PrimaryM float64
}

// exhaustPath is one exhaust path. The pipe and outlet radii and the
// chamber are the EngineLab path stages of the earlier model, which the
// profile no longer has. The pop bank follows them.
type exhaustPath struct {
	CollectorM, PipeRadiusM, OutletRadiusM float64
	ChamberM, ChamberArea, Gain            float64
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "import:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	esDir := fs.String("engine-sim", "", "`directory` of engine-sim JSON dumps")
	elDir := fs.String("enginelab", "", "`directory` of EngineLab JSON files: engines/*.json and exhausts.json")
	profiles := fs.String("profiles", "", "output `directory` for the profiles and README.md")
	scenarios := fs.String("scenarios", "", "output `directory` for the scenarios")
	library := fs.String("sound-library", "", "engine-sim sound library `directory` with the impulse responses (default: share/engine-sim/es/sound-library of nix build -f reference/engine-sim)")
	reference := fs.String("reference", "", "write the reference set to this `directory`, which holds its cycle tables in cycles/, in place of the catalogue")
	ro := refOptions{refs: map[string]string{}}
	fs.StringVar(&ro.grid, "grid", "out/cycles/grid", "with --reference, `directory` of the grid records of just cycles-grid, which the sources cite")
	fs.StringVar(&ro.share, "engine-sim-share", "", "with --reference, share/engine-sim `directory` of the engine-sim package (default: that of nix build -f reference/engine-sim)")
	fs.Func("ref", "with --reference, `id=directory` of the reference renders of one engine, in place of its default (repeatable)", func(s string) error {
		id, dir, ok := strings.Cut(s, "=")
		if !ok || dir == "" || !slices.ContainsFunc(referenceEngines, func(r refEngine) bool { return r.id == id }) {
			return fmt.Errorf("%q is not id=directory of a reference engine", s)
		}
		ro.refs[id] = dir
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *reference != "" {
		if *scenarios == "" || fs.NArg() != 0 {
			return errors.New("--reference needs --scenarios, and no other arguments")
		}
		ro.out, ro.scenarios = *reference, *scenarios
		return runReference(&ro)
	}
	if *esDir == "" || *elDir == "" || *profiles == "" || *scenarios == "" {
		fs.Usage()
		return errors.New("all four directories are required")
	}
	lib, err := soundLibrary(*library)
	if err != nil {
		return err
	}
	es, err := loadEngineSim(*esDir)
	if err != nil {
		return err
	}
	el, err := loadEngineLab(*elDir)
	if err != nil {
		return err
	}
	all := slices.Concat(es, el)
	if err := checkIDs(all); err != nil {
		return err
	}
	if err := fitBanks(all, lib); err != nil {
		return err
	}
	for _, dir := range []string{*profiles, *scenarios} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for i := range all {
		e := &all[i]
		e.PulseTable = pulseFor(len(e.Cylinders))
		// The catalogue has no cycle tables, so snarl cannot render it, and
		// its output level is not calibrated.
		if err := os.WriteFile(filepath.Join(*profiles, e.ID+".toml"), []byte(profileTOML(e)), 0o644); err != nil {
			return err
		}
		if err := writeScenarios(filepath.Join(*scenarios, e.ID), e); err != nil {
			return err
		}
		fmt.Printf("%-34s %2d cylinders, %d banks\n", e.ID, len(e.Cylinders), len(e.Paths))
	}
	if err := writeScripts(filepath.Join(*scenarios, "engine-sim-scripts.txt"), all); err != nil {
		return err
	}
	return writeReadme(filepath.Join(*profiles, "README.md"), all)
}

// checkIDs reports a missing or repeated identifier, or one that names a
// maker or a model.
func checkIDs(all []engine) error {
	seen := map[string]bool{}
	for _, e := range all {
		if e.ID == "" {
			return fmt.Errorf("%s %s has no identifier in the naming table", e.Project, e.File)
		}
		if seen[e.ID] {
			return fmt.Errorf("identifier %s is used twice", e.ID)
		}
		seen[e.ID] = true
		for _, word := range forbiddenWords {
			if strings.Contains(e.ID, word) || strings.Contains(strings.ToLower(e.Description), word) {
				return fmt.Errorf("%s: identifier or description contains %q", e.ID, word)
			}
		}
	}
	return nil
}

// forbiddenWords are the maker and model names in the upstream file names.
var forbiddenWords = []string{
	"honda", "kohler", "harley", "davidson", "shovelhead", "hayabusa", "suzuki", "subaru", "audi", "toyota",
	"2jz", "gm", "ls3", "chev", "ferrari", "lfa", "lexus", "merlin", "bmw", "porsche", "yamaha", "vw",
	"tdi", "vtec", "k20", "ej25", "m52", "f136", "cp2", "cp3", "cp4", "mt07", "mt09", "mt10", "rd350",
}

// writeScripts writes the engine-sim script of each engine that comes from
// engine-sim, as "id script" lines, for just ref-set catalogue.
func writeScripts(path string, all []engine) error {
	var b strings.Builder
	b.WriteString("# Generated by just import-profiles: catalogue id and engine-sim script for just ref-set catalogue.\n")
	for _, e := range all {
		if e.Project == "engine-sim" {
			fmt.Fprintf(&b, "%s %s\n", e.ID, e.File)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// banksInOrder renumbers keys to banks from 0 in the order of first use.
func banksInOrder[K comparable](keys []K) ([]int, []K) {
	var order []K
	banks := make([]int, len(keys))
	for i, k := range keys {
		j := slices.Index(order, k)
		if j < 0 {
			j = len(order)
			order = append(order, k)
		}
		banks[i] = j
	}
	return banks, order
}
