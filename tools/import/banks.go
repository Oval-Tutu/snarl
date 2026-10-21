package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// engineSimTaps is the most taps that engine-sim convolves with.
	engineSimTaps = 10000
	// legacyTaps is the length of the impulse response of the pop stages.
	legacyTaps = 1 << 16
	// defaultIR is the impulse response of the EngineLab profiles: the
	// engine-sim default impulse response (default_0 of the impulse
	// response library).
	defaultIR = "smooth_39.wav"
	// hfRetryDB is the largest third-octave error from hfRetryLo to
	// hfRetryHi of a formant bank with the -40 dB floor. Above it, the bank
	// is fitted again with the -60 dB floor, so that it follows the
	// response where the input jitter puts its noise. The check stops at
	// 10 kHz, because the engine-sim output is dark above about 15 kHz.
	hfRetryDB = 3
	hfRetryLo = 6000
	hfRetryHi = 10000
)

// The engine-sim synthesiser defaults (Synthesizer::AudioParameters and
// the engine node of es/objects/objects.mr), for the EngineLab profiles.
const (
	defaultHFGain = 0.01
	defaultNoise  = 1.0
	defaultJitter = 0.5
)

// irFit and popFit are the fit settings of tools/fit for the formant bank
// (--base ir --hi 16000) and the pop bank (--pop --lo 10 --hi 16000
// --weight octave --floor -60).
var (
	irFit = analysis.BankFit{
		Lo: 40, Hi: 16000, PerOctave: 24, Smooth: 6, FloorDB: -40,
		MinN: 4, MaxN: 12, Tolerance: 1, Rate: rate,
	}
	popFit = analysis.BankFit{
		Lo: 10, Hi: 16000, PerOctave: 24, Smooth: 6, FloorDB: -60, EqualWeights: true,
		MinN: 4, MaxN: 12, Tolerance: 1, Rate: rate,
	}
)

// The defaults of the EngineLab path stages of the earlier model that the
// pop bank follows: the exhaust and path values that every imported
// profile had.
var legacyExhaust = analysis.LegacyExhaust{SpeedOfSound: 565, OutletDirect: 0.4, LowShelfHz: 250}

const (
	legacyWallLossDB = 2.5
	legacyWallLossHz = 2000
)

// soundLibrary returns the engine-sim sound library directory: dir, or
// the library of the reference engine-sim package.
func soundLibrary(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	out, err := exec.CommandContext(context.Background(), "nix", "build", "-f", "reference/engine-sim", "--print-out-paths", "--no-link").Output()
	if err != nil {
		return "", fmt.Errorf("nix build -f reference/engine-sim: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "share", "engine-sim", "es", "sound-library"), nil
}

// findIR returns the path of the impulse response file with the base name
// in the sound library.
func findIR(lib, name string) (string, error) {
	var found string
	err := filepath.WalkDir(lib, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name && found == "" {
			found = p
		}
		return err
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no impulse response %s in %s", name, lib)
	}
	return found, nil
}

// fitIR returns the formant bank of the impulse response file, fitted as
// tools/fit fits it, with its gain set so that its highest response on the
// grid is 0 dB.
func fitIR(path string) (analysis.FormantBank, error) {
	audio, err := wav.ReadFile(path)
	if err != nil {
		return analysis.FormantBank{}, err
	}
	ir := analysis.IRTrim(audio.Mono(), engineSimTaps)
	if len(ir) == 0 {
		return analysis.FormantBank{}, fmt.Errorf("%s: no sample above the engine-sim threshold", path)
	}
	fit := irFit
	b, err := chosenBank(&fit, ir, audio.SampleRate)
	if err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if b.BandMaxErrorDB(ir, audio.SampleRate, rate, hfRetryLo, hfRetryHi) > hfRetryDB {
		fit.FloorDB = -60
		if b, err = chosenBank(&fit, ir, audio.SampleRate); err != nil {
			return b, fmt.Errorf("%s: %w", path, err)
		}
	}
	b.NormalisePeak(fit.Grid(), fit.Rate)
	return b, nil
}

// chosenBank fits the response and returns the bank of the chosen count.
func chosenBank(fit *analysis.BankFit, h []float64, hRate int) (analysis.FormantBank, error) {
	banks, _, n, _ := fit.Fit(h, hRate)
	if n == 0 {
		return analysis.FormantBank{}, errors.New("the fit stopped early")
	}
	return banks[n-1], nil
}

// legacyPaths returns the EngineLab pop stages of the earlier model for the
// paths of e. The collector is left out, because the model keeps it.
func legacyPaths(e *engine) []analysis.LegacyPath {
	out := make([]analysis.LegacyPath, len(e.Paths))
	for i, p := range e.Paths {
		out[i] = analysis.LegacyPath{
			PipeRadiusM: p.PipeRadiusM, OutletM: p.OutletRadiusM,
			WallLossDB: legacyWallLossDB, WallLossHz: legacyWallLossHz,
			ChamberM: p.ChamberM, ChamberArea: p.ChamberArea,
		}
	}
	return out
}

// popKey returns the pop stages of the first path of e, which the pop
// bank follows, and a note when another path has other stages.
func popKey(e *engine) (analysis.LegacyPath, string) {
	paths := legacyPaths(e)
	for _, p := range paths[1:] {
		if p != paths[0] {
			return paths[0], "The pop bank follows the pipe, outlet and silencer of the first exhaust path."
		}
	}
	return paths[0], ""
}

// fitPop returns the pop bank of the pop stages of the path, with its
// fitted gain, and the pop delay in milliseconds.
func fitPop(p analysis.LegacyPath) (analysis.FormantBank, float64, error) {
	h := analysis.LegacyPathImpulse(&p, &legacyExhaust, rate, legacyTaps, analysis.LegacyPopStages())
	fit := popFit
	b, err := chosenBank(&fit, h, rate)
	return b, analysis.LegacyPopDelayMS(&p, &legacyExhaust), err
}

// fitBanks fits the formant bank of each engine from its impulse response
// and the pop bank from the EngineLab path stages that the profile had.
// Each distinct impulse response and pop stage set is fitted once, and the
// fits run in parallel. The fits are deterministic, so the order does not
// change the result.
func fitBanks(all []engine, lib string) error {
	type popFitResult struct {
		bank  analysis.FormantBank
		delay float64
	}
	irs := map[string]analysis.FormantBank{}
	pops := map[analysis.LegacyPath]popFitResult{}
	keys := make([]analysis.LegacyPath, len(all))
	var irNames []string
	var popKeys []analysis.LegacyPath
	for i := range all {
		e := &all[i]
		if e.ImpulseResponse == "" {
			e.ImpulseResponse = defaultIR
		}
		if _, ok := irs[e.ImpulseResponse]; !ok {
			irs[e.ImpulseResponse] = analysis.FormantBank{}
			irNames = append(irNames, e.ImpulseResponse)
		}
		k, note := popKey(e)
		keys[i] = k
		if _, ok := pops[k]; !ok {
			pops[k] = popFitResult{}
			popKeys = append(popKeys, k)
		}
		if note != "" {
			e.Notes = append(e.Notes, note)
		}
	}
	// The goroutines write the maps, so the loops range over the key
	// slices: a range over a map that another goroutine writes is a fatal
	// error.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, name := range irNames {
		wg.Go(func() {
			var b analysis.FormantBank
			path, err := findIR(lib, name)
			if err == nil {
				b, err = fitIR(path)
			}
			mu.Lock()
			defer mu.Unlock()
			irs[name] = b
			if err != nil {
				errs = append(errs, err)
			}
		})
	}
	for _, k := range popKeys {
		wg.Go(func() {
			b, d, err := fitPop(k)
			mu.Lock()
			defer mu.Unlock()
			pops[k] = popFitResult{b, d}
			if err != nil {
				errs = append(errs, err)
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for i := range all {
		e := &all[i]
		e.Bank = irs[e.ImpulseResponse]
		e.PopBank, e.PopDelayMS = pops[keys[i]].bank, pops[keys[i]].delay
	}
	return nil
}

// bankTOML writes the sections of the bank as [[table]] entries.
func bankTOML(b *strings.Builder, table string, bank *analysis.FormantBank) {
	for _, f := range bank.Formants {
		kind := f.Kind
		if kind == analysis.KindPeak {
			kind = "peak"
		}
		fmt.Fprintf(b, "\n[[%s]]\nkind = %q\nhz = %s\nq = %s\n", table, kind, num(round(f.Hz, 1)), num(round(f.Q, 3)))
		if f.Kind == analysis.KindPeak {
			fmt.Fprintf(b, "gain_db = %s\n", num(round(f.GainDB, 2)))
		}
	}
}
