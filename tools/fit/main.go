// Command fit fits a cascade of biquads (a formant bank: peaking
// sections, with at most one low pass and one high pass) to the magnitude
// response of an engine-sim impulse response, for N sections from --min
// to --max, and writes the fitted parameters of the smallest N whose
// error is at most --tolerance dB (or of the N with the lowest error,
// when none is) as a proposed TOML fragment. The parameters are snarl's
// own: centre frequencies, Qs and gains that follow the measured
// response, not taps of the response.
//
// The error is the weighted RMS difference in dB on a 1/24-octave grid
// from --lo to --hi, smoothed over 1/--smooth octave (default 1/6) or
// over one equivalent rectangular bandwidth (ERB) with --smooth 0, with
// each grid point weighted by the number of ERBs per octave. Below 500 Hz
// an ERB is wider than a third octave and blurs the formant peaks that
// decide the strongest engine order, so 1/6 octave is the default. Levels
// of
// the engine-sim harmonic path (the 1.9 kHz input low pass, the
// derivative and the response) more than --floor dB below its loudest
// grid point pass through a soft floor, so that inaudible valleys count
// little.
//
// With --profile, the report also gives the response of each linear
// stage of each exhaust path of that snarl profile, and the error of the
// whole path against the same target.
//
// Usage:
//
//	go run ./tools/fit --ir IR.wav [--profile profiles/v8.toml] [--out out/analysis/fit/v8.toml] > fit.md
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

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// engineSimTaps is the most taps that engine-sim convolves with.
	engineSimTaps = 10000
	// inputLowPassHz is the cut-off of the engine-sim input low pass.
	inputLowPassHz = 1900
	// gridPerOctave is the number of grid points per octave.
	gridPerOctave = 24
	// pathTaps is the length of the impulse response of a snarl path.
	pathTaps = 1 << 16
)

type config struct {
	ir, profile, out           string
	lo, hi, floor, tol, smooth float64
	minN, maxN, rate           int
	name                       string
}

// smoothing names the smoothing of the target.
func (c *config) smoothing() string {
	if c.smooth > 0 {
		return fmt.Sprintf("1/%g octave", c.smooth)
	}
	return "one ERB"
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fit:", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	var cfg config
	fs := flag.NewFlagSet("fit", flag.ContinueOnError)
	fs.StringVar(&cfg.ir, "ir", "", "engine-sim impulse response WAV `file`")
	fs.StringVar(&cfg.profile, "profile", "", "snarl profile TOML `file` whose exhaust paths the report compares")
	fs.StringVar(&cfg.out, "out", "", "TOML `file` for the proposed fragment (none when empty)")
	fs.StringVar(&cfg.name, "name", "", "profile name in the fragment (default: the stem of --profile or --ir)")
	fs.Float64Var(&cfg.lo, "lo", 40, "lowest grid frequency in `Hz`")
	fs.Float64Var(&cfg.hi, "hi", 6000, "highest grid frequency in `Hz`")
	fs.Float64Var(&cfg.floor, "floor", -40, "soft floor in `dB` below the loudest point of the harmonic path (0 turns it off)")
	fs.Float64Var(&cfg.tol, "tolerance", 1, "weighted error in `dB` that the chosen N must not exceed")
	fs.Float64Var(&cfg.smooth, "smooth", 6, "smoothing width of the target as 1/`N` octave (0 smooths over one ERB)")
	fs.IntVar(&cfg.minN, "min", 4, "fewest sections in the report")
	fs.IntVar(&cfg.maxN, "max", 12, "most sections")
	fs.IntVar(&cfg.rate, "rate", 48000, "sample rate in `Hz` that the bank runs at")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.ir == "" || fs.NArg() != 0 {
		fs.Usage()
		return errors.New("need --ir and no other arguments")
	}
	if cfg.minN < 1 || cfg.maxN < cfg.minN || cfg.lo <= 0 || cfg.hi <= cfg.lo || cfg.rate < 8000 {
		return errors.New("need 1 <= --min <= --max, 0 < --lo < --hi and --rate from 8000")
	}
	if cfg.name == "" {
		src := cfg.ir
		if cfg.profile != "" {
			src = cfg.profile
		}
		cfg.name = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	audio, err := wav.ReadFile(cfg.ir)
	if err != nil {
		return err
	}
	irRate := audio.SampleRate
	ir := analysis.IRTrim(audio.Mono(), engineSimTaps)
	if len(ir) == 0 {
		return fmt.Errorf("%s: no sample above the engine-sim threshold", cfg.ir)
	}
	hz := analysis.LogGrid(cfg.lo, cfg.hi, gridPerOctave)
	newTarget := func() *analysis.FormantTarget {
		return &analysis.FormantTarget{
			Hz:      hz,
			DB:      analysis.SmoothedResponse(ir, irRate, hz, cfg.smooth),
			BaseDB:  analysis.HarmonicBaseDB(hz, float64(irRate), inputLowPassHz),
			Weight:  analysis.ERBWeights(hz),
			FloorDB: cfg.floor,
			Rate:    float64(cfg.rate),
		}
	}
	target := newTarget()
	banks, errs := analysis.FitFormants(target, cfg.maxN)
	if len(banks) < cfg.maxN {
		return errors.New("the fit stopped early")
	}
	chosen, met := chooseN(errs, cfg.minN, cfg.tol)
	bank := banks[chosen-1]
	normalise(&bank, hz, float64(cfg.rate))

	fmt.Fprintf(w, "## %s\n\nImpulse response `%s`: %d taps at %d Hz as engine-sim loads it. Grid: %g Hz to %g Hz, %d points per octave, smoothed over %s, ERB weights, soft floor %g dB below the loudest point of the harmonic path. The bank runs at %d Hz.\n\n",
		cfg.name, cfg.ir, len(ir), irRate, cfg.lo, cfg.hi, gridPerOctave, cfg.smoothing(), cfg.floor, cfg.rate)
	bands := thirdOctaves(cfg.lo, cfg.hi)
	irBands := analysis.IRResponse(ir, irRate, bands)
	base := bandBase(bands, float64(irRate))
	fmt.Fprint(w, "| Sections | Weighted error (dB) | Third-octave RMS 50-2000 Hz (dB) | Third-octave max 50-6000 Hz (dB) | Multiply-adds per sample |\n| --- | --- | --- | --- | --- |\n")
	for n := cfg.minN; n <= cfg.maxN; n++ {
		b := banks[n-1]
		r, mx := bandErrors(bands, irBands, base, bankBands(&b, bands, float64(cfg.rate)), cfg.floor)
		mark := ""
		if n == chosen {
			mark = " (chosen)"
		}
		fmt.Fprintf(w, "| %d%s | %.2f | %.2f | %.2f | %d |\n", n, mark, errs[n-1], r, mx, b.MultiplyAdds())
	}
	if met {
		fmt.Fprintf(w, "\nChosen: %d sections, the smallest N from %d to %d with a weighted error of at most %g dB.\n\n", chosen, cfg.minN, cfg.maxN, cfg.tol)
	} else {
		fmt.Fprintf(w, "\nChosen: %d sections. No N from %d to %d has a weighted error of at most %g dB, so this is the N with the lowest error.\n\n", chosen, cfg.minN, cfg.maxN, cfg.tol)
	}
	fmt.Fprintf(w, "| Section | Kind | Frequency (Hz) | Q | Gain (dB) |\n| --- | --- | --- | --- | --- |\n")
	for i, f := range bank.Formants {
		fmt.Fprintf(w, "| %d | %s | %.1f | %.2f | %.2f |\n", i+1, kindName(f.Kind), f.Hz, f.Q, f.GainDB)
	}
	fmt.Fprintf(w, "| Gain | | | | %.2f |\n\n", bank.GainDB)

	var paths []pathSpec
	var exhaust exhaustSpec
	if cfg.profile != "" {
		f, err := os.Open(cfg.profile)
		if err != nil {
			return err
		}
		exhaust, paths, err = readExhaust(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", cfg.profile, err)
		}
	}
	responseTable(w, bands, irBands, base, &bank, paths, &exhaust, cfg.rate)
	if len(paths) > 0 {
		stageTable(w, bands, paths, &exhaust, cfg.rate, cfg.smooth, newTarget)
	}
	if cfg.out != "" {
		if err := writeFragment(cfg.out, &cfg, &bank, errs[chosen-1], len(paths)); err != nil {
			return err
		}
		fmt.Fprintf(w, "Wrote `%s`.\n\n", cfg.out)
	}
	return nil
}

// kindName returns the TOML name of a section kind.
func kindName(k string) string {
	if k == analysis.KindPeak {
		return "peak"
	}
	return k
}

// chooseN returns the smallest number of sections from minN whose error
// errs[n-1] is at most tol, and true. When none is, it returns the number
// with the lowest error from minN, and false.
func chooseN(errs []float64, minN int, tol float64) (n int, met bool) {
	best, bestErr := 0, math.Inf(1)
	for k := minN; k <= len(errs); k++ {
		e := errs[k-1]
		if e <= tol {
			return k, true
		}
		if e < bestErr {
			best, bestErr = k, e
		}
	}
	return best, false
}

// normalise moves the gain of the bank so that its highest response on
// the grid is 0 dB.
func normalise(b *analysis.FormantBank, hz []float64, rate float64) {
	peak := math.Inf(-1)
	for _, f := range hz {
		peak = math.Max(peak, b.ResponseDB(f, rate))
	}
	b.GainDB -= peak
}

// thirdOctaves returns the third-octave bands of the analysis whose
// centre lies from lo to hi.
func thirdOctaves(lo, hi float64) [][2]float64 {
	var out [][2]float64
	for _, b := range analysis.ThirdOctaveBands(48000) {
		c := math.Sqrt(b[0] * b[1])
		if c >= lo*0.99 && c <= hi*1.01 {
			out = append(out, b)
		}
	}
	return out
}

// bandBase returns the response of the harmonic path without its impulse
// response in each band, at the centre of the band.
func bandBase(bands [][2]float64, rate float64) []float64 {
	c := make([]float64, len(bands))
	for i, b := range bands {
		c[i] = math.Sqrt(b[0] * b[1])
	}
	return analysis.HarmonicBaseDB(c, rate, inputLowPassHz)
}

// bankBands returns the power response in dB of the bank in each band, as
// the mean power over 64 points equally spaced in the band.
func bankBands(b *analysis.FormantBank, bands [][2]float64, rate float64) []float64 {
	out := make([]float64, len(bands))
	for i, band := range bands {
		var s float64
		for k := range 64 {
			f := band[0] + (band[1]-band[0])*(float64(k)+0.5)/64
			s += math.Pow(10, b.ResponseDB(f, rate)/10)
		}
		out[i] = 10 * math.Log10(s/64)
	}
	return out
}

// bandErrors returns the RMS of the band differences from 50 Hz to 2 kHz
// and the largest band difference from 50 Hz to 6 kHz between the
// response m and the reference ref, after the offset that matches their
// means from 50 Hz to 2 kHz. The largest difference leaves out bands
// whose harmonic path level (ref plus base) is below the floor.
func bandErrors(bands [][2]float64, ref, base, m []float64, floor float64) (rmsDB, maxDB float64) {
	var off float64
	n := 0
	for i, b := range bands {
		if c := math.Sqrt(b[0] * b[1]); c >= 50 && c <= 2000 {
			off += ref[i] - m[i]
			n++
		}
	}
	off /= float64(max(n, 1))
	peak := math.Inf(-1)
	for i := range bands {
		peak = math.Max(peak, ref[i]+base[i])
	}
	var s float64
	for i, b := range bands {
		c := math.Sqrt(b[0] * b[1])
		d := m[i] + off - ref[i]
		if c >= 50 && c <= 2000 {
			s += d * d
		}
		if c >= 50 && c <= 6000 && (floor >= 0 || ref[i]+base[i]-peak >= floor) {
			maxDB = math.Max(maxDB, math.Abs(d))
		}
	}
	return math.Sqrt(s / float64(max(n, 1))), maxDB
}

// responseTable writes the harmonic path of engine-sim, with the fitted
// bank in place of the impulse response, and with each snarl path in
// place of it, per band, relative to the highest band of engine-sim.
func responseTable(w io.Writer, bands [][2]float64, irBands, base []float64, bank *analysis.FormantBank, paths []pathSpec, e *exhaustSpec, rate int) {
	cols := [][]float64{irBands, bankBands(bank, bands, float64(rate))}
	names := []string{"engine-sim", "Fitted bank"}
	for i := range paths {
		h := pathImpulse(&paths[i], e, float64(rate), pathTaps, allStages())
		cols = append(cols, analysis.IRResponse(h, rate, bands))
		names = append(names, fmt.Sprintf("snarl path %d", i+1))
	}
	// Each column moves so that its mean from 50 Hz to 2 kHz matches
	// engine-sim, then all are relative to the highest engine-sim band.
	peak := math.Inf(-1)
	for i := range bands {
		peak = math.Max(peak, irBands[i]+base[i])
	}
	fmt.Fprint(w, "Harmonic path (the 1.9 kHz input low pass, the derivative and the named response), in dB relative to the highest engine-sim band. Each column after the first has the offset that matches its mean from 50 Hz to 2 kHz to engine-sim.\n\n| Band (Hz) |")
	for _, n := range names {
		fmt.Fprintf(w, " %s |", n)
	}
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+len(names))+"\n")
	offs := make([]float64, len(cols))
	for k, c := range cols {
		var s float64
		n := 0
		for i, b := range bands {
			if cc := math.Sqrt(b[0] * b[1]); cc >= 50 && cc <= 2000 {
				s += irBands[i] - c[i]
				n++
			}
		}
		offs[k] = s / float64(max(n, 1))
	}
	for i, b := range bands {
		fmt.Fprintf(w, "| %.0f |", math.Sqrt(b[0]*b[1]))
		for k, c := range cols {
			fmt.Fprintf(w, " %.1f |", c[i]+offs[k]+base[i]-peak)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, "| RMS difference 50-2000 Hz |")
	for _, c := range cols {
		r, _ := bandErrors(bands, irBands, base, c, 0)
		fmt.Fprintf(w, " %.2f |", r)
	}
	fmt.Fprint(w, "\n\n")
}

// stageTable writes the response of each linear stage of each path alone,
// per band relative to its own mean from 50 Hz to 2 kHz, its cost, and
// the weighted error of the whole path against the fit target.
func stageTable(w io.Writer, bands [][2]float64, paths []pathSpec, e *exhaustSpec, rate int, smooth float64, newTarget func() *analysis.FormantTarget) {
	for i := range paths {
		fmt.Fprintf(w, "snarl path %d, each linear stage alone, in dB relative to its mean from 50 Hz to 2 kHz.\n\n| Band (Hz) |", i+1)
		var cols [][]float64
		for s := range stageCount {
			fmt.Fprintf(w, " %s |", stageNames[s])
			h := pathImpulse(&paths[i], e, float64(rate), pathTaps, onlyStage(s))
			cols = append(cols, analysis.IRResponse(h, rate, bands))
		}
		fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+int(stageCount))+"\n")
		means := make([]float64, len(cols))
		for k, c := range cols {
			n := 0
			for j, b := range bands {
				if cc := math.Sqrt(b[0] * b[1]); cc >= 50 && cc <= 2000 {
					means[k] += c[j]
					n++
				}
			}
			means[k] /= float64(max(n, 1))
		}
		for j, b := range bands {
			fmt.Fprintf(w, "| %.0f |", math.Sqrt(b[0]*b[1]))
			for k, c := range cols {
				fmt.Fprintf(w, " %.1f |", c[j]-means[k])
			}
			fmt.Fprintln(w)
		}
		fmt.Fprint(w, "| Multiply-adds |")
		total := 0
		for s := range stageCount {
			fmt.Fprintf(w, " %d |", stageCost[s])
			total += stageCost[s]
		}
		t := newTarget()
		h := pathImpulse(&paths[i], e, float64(rate), pathTaps, allStages())
		pe, _ := analysis.ShapeError(t, analysis.SmoothedResponse(h, rate, t.Hz, smooth))
		fmt.Fprintf(w, "\n\nThe linear stages of path %d cost %d multiply-adds per sample. The whole path is %.2f dB from the fit target (weighted error, best gain).\n\n", i+1, total, pe)
	}
}

// writeFragment writes the proposed TOML fragment for the bank.
func writeFragment(path string, cfg *config, b *analysis.FormantBank, errDB float64, paths int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var s strings.Builder
	fmt.Fprintf(&s, "# Proposed formant bank for the %s profile, written by tools/fit.\n", cfg.name)
	fmt.Fprintf(&s, "# A cascade of %d biquads (Audio EQ Cookbook peaking, low pass and high\n# pass sections) and one gain, fitted\n", len(b.Formants))
	fmt.Fprintf(&s, "# to the magnitude of the engine-sim impulse response %s.\n", filepath.Base(cfg.ir))
	fmt.Fprintf(&s, "# The values are snarl's own fit parameters, not taps of the response.\n")
	fmt.Fprintf(&s, "# Weighted error %.2f dB from %g Hz to %g Hz (smoothed over %s, ERB\n# weights, soft floor", errDB, cfg.lo, cfg.hi, cfg.smoothing())
	fmt.Fprintf(&s, " %g dB below the loudest point of the harmonic path).\n# The gain sets the", cfg.floor)
	fmt.Fprintf(&s, " highest response of the bank on the grid to 0 dB.\n")
	if paths > 0 {
		fmt.Fprintf(&s, "# engine-sim uses one impulse response for every exhaust system, so one\n")
		fmt.Fprintf(&s, "# bank serves every path of the profile (%d).\n", paths)
	}
	fmt.Fprintf(&s, "# No model code reads these keys yet (docs/research/formant-fit.md).\n\n")
	fmt.Fprintf(&s, "[exhaust]\nformant_gain_db = %.2f\n", b.GainDB)
	for _, f := range b.Formants {
		fmt.Fprintf(&s, "\n[[formant]]\nkind = %q\nhz = %.1f\nq = %.3f\n", kindName(f.Kind), f.Hz, f.Q)
		if f.Kind == analysis.KindPeak {
			fmt.Fprintf(&s, "gain_db = %.2f\n", f.GainDB)
		}
	}
	return os.WriteFile(path, []byte(s.String()), 0o644)
}
