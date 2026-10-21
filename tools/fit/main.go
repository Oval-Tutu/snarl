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
// more than --floor dB below the loudest grid point pass through a soft
// floor, so that inaudible valleys count little. With --base harmonic,
// the levels are those of the engine-sim harmonic path (the 1.9 kHz input
// low pass, the derivative and the response). With --base ir, they are
// those of the response alone, which also passes the noise and the input
// jitter of engine-sim.
//
// With --profile, the report also gives the response of each linear
// stage of each exhaust path of that profile, which must have the
// EngineLab path keys of a73b874, and the error of the whole path against
// the same target. With --pop, the target is not an impulse response but
// the response of the pop stages of that profile at a73b874: the wall
// loss, the low shelf, the expansion chamber, the mode cutoff and the
// outlet of its first path, which every path shares. The pop bank keeps
// its fitted gain, so that the pop crack keeps its balance with the pop
// body.
//
// With --pulse, it measures instead the engine-sim runner pressure pulse
// of one firing in each hold of the CSV files given as arguments (rendered
// with one row per step), as the sum of the runner pressures of each
// exhaust system that the engine dump of --engine names, with the scenario
// of the same name in --scenarios. The report gives the shape of the pulse
// of each hold, the error of one table whose length follows the RPM, and
// the error of one row per hold RPM, at the holds and left out. With --out,
// it writes the rows of the holds that --throttle, --min-rpm and --max-rpm
// select as the pulse file that the model embeds
// (internal/model/pulses/<profile>.txt).
//
// With --pulse --variation, it also measures the cycle-to-cycle variation
// of each hold: with the pulse fixed, it fits a gain, a shift and a width
// to each firing in turn, fits the pulse again with them, and reports the
// spread of each over the firings about the mean of their cylinder, the
// spread of the means of the cylinders, and the residuals of the sums and
// of the part that does not repeat from cycle to cycle. With --out, the
// pulse file gets one spread line per row with the firing gain spread,
// which the model takes with [pulse] measured_variation.
//
// Usage:
//
//	go run ./tools/fit --ir IR.wav [--base ir] [--profile LEGACY.toml] [--out out/analysis/fit/v8.toml] > fit.md
//	go run ./tools/fit --pop --profile LEGACY.toml [--out out/analysis/fit/v8-pop.toml] > fit.md
//	go run ./tools/fit --pulse [--variation] --throttle 1 --engine engine.json --scenarios scenarios/v8 [--out internal/model/pulses/v8.txt] holds-t100.csv [holds-t050.csv] > pulse.md
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
	ir, profile, out, base     string
	weight                     string
	lo, hi, floor, tol, smooth float64
	minN, maxN, rate           int
	name                       string
	pop                        bool
	// The pulse fit: --pulse, the engine dump, the scenario directory, the
	// table points, and the throttle and lowest RPM of the fitted holds.
	pulse             bool
	engine, scenarios string
	points            int
	throttle, minRPM  float64
	maxRPM            float64
	// variation adds the cycle-to-cycle variation of each hold.
	variation bool
}

// smoothing names the smoothing of the target.
func (c *config) smoothing() string {
	if c.smooth > 0 {
		return fmt.Sprintf("1/%g octave", c.smooth)
	}
	return "one ERB"
}

// weightText names the weights of the grid points.
func (c *config) weightText() string {
	if c.weight == "octave" {
		return "equal"
	}
	return "ERB"
}

// harmonic reports whether the floor follows the engine-sim harmonic path.
func (c *config) harmonic() bool {
	return !c.pop && c.base == "harmonic"
}

// floorText names the level that the soft floor is relative to.
func (c *config) floorText() string {
	if c.harmonic() {
		return "the loudest point of the harmonic path"
	}
	return "the loudest point of the response"
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
	fs.StringVar(&cfg.profile, "profile", "", "profile TOML `file` with the EngineLab path keys of a73b874, whose exhaust paths the report compares")
	fs.BoolVar(&cfg.pop, "pop", false, "fit the pop stages of --profile in place of --ir")
	fs.StringVar(&cfg.base, "base", "harmonic", "levels of the soft floor: harmonic (the engine-sim harmonic path) or ir (the response alone)")
	fs.StringVar(&cfg.weight, "weight", "erb", "weight of each grid point: erb (the ERBs per octave) or octave (the same for every point, which gives the low frequencies of a pop more weight)")
	fs.StringVar(&cfg.out, "out", "", "TOML `file` for the proposed fragment (none when empty)")
	fs.StringVar(&cfg.name, "name", "", "profile name in the fragment (default: the stem of --profile or --ir)")
	fs.Float64Var(&cfg.lo, "lo", 40, "lowest grid frequency in `Hz`")
	fs.Float64Var(&cfg.hi, "hi", 6000, "highest grid frequency in `Hz`")
	fs.Float64Var(&cfg.floor, "floor", -40, "soft floor in `dB` below the loudest point of the harmonic path or the response (0 turns it off)")
	fs.Float64Var(&cfg.tol, "tolerance", 1, "weighted error in `dB` that the chosen N must not exceed")
	fs.Float64Var(&cfg.smooth, "smooth", 6, "smoothing width of the target as 1/`N` octave (0 smooths over one ERB)")
	fs.IntVar(&cfg.minN, "min", 4, "fewest sections in the report")
	fs.IntVar(&cfg.maxN, "max", 12, "most sections")
	fs.IntVar(&cfg.rate, "rate", 48000, "sample rate in `Hz` that the bank runs at")
	fs.BoolVar(&cfg.pulse, "pulse", false, "measure the runner pressure pulse of the engine-sim CSV files given as arguments, in place of a bank fit")
	fs.StringVar(&cfg.engine, "engine", "", "engine-sim-render --dump-json `file` of the engine of the --pulse CSV files")
	fs.StringVar(&cfg.scenarios, "scenarios", "", "`directory` with the scenario of each --pulse CSV file, of the same name")
	fs.IntVar(&cfg.points, "points", 128, "number of values of the --pulse table")
	fs.Float64Var(&cfg.throttle, "throttle", 0.5, "lowest throttle of the holds that the --pulse table follows")
	fs.Float64Var(&cfg.minRPM, "min-rpm", 0, "lowest RPM of the holds that the --pulse table follows")
	fs.Float64Var(&cfg.maxRPM, "max-rpm", 100000, "highest RPM of the holds that the --pulse table follows")
	fs.BoolVar(&cfg.variation, "variation", false, "with --pulse, also fit a gain, a shift and a width to each firing and report their spreads")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.pulse {
		if cfg.name == "" {
			cfg.name = filepath.Base(cfg.scenarios)
		}
		return runPulse(&cfg, fs.Args(), w)
	}
	switch {
	case fs.NArg() != 0 || (cfg.ir == "") == !cfg.pop || cfg.pop && cfg.profile == "":
		fs.Usage()
		return errors.New("need --ir, or --pop with --profile, and no other arguments")
	case cfg.minN < 1 || cfg.maxN < cfg.minN || cfg.lo <= 0 || cfg.hi <= cfg.lo || cfg.rate < 8000:
		return errors.New("need 1 <= --min <= --max, 0 < --lo < --hi and --rate from 8000")
	case cfg.base != "harmonic" && cfg.base != "ir":
		return fmt.Errorf("--base %q is not harmonic or ir", cfg.base)
	case cfg.weight != "erb" && cfg.weight != "octave":
		return fmt.Errorf("--weight %q is not erb or octave", cfg.weight)
	}
	if cfg.name == "" {
		src := cfg.ir
		if cfg.profile != "" {
			src = cfg.profile
		}
		cfg.name = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	var paths []analysis.LegacyPath
	var exhaust analysis.LegacyExhaust
	if cfg.profile != "" {
		f, err := os.Open(cfg.profile)
		if err != nil {
			return err
		}
		exhaust, paths, err = analysis.ReadLegacyExhaust(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", cfg.profile, err)
		}
	}
	ir, irRate, source, err := loadTarget(&cfg, paths, &exhaust)
	if err != nil {
		return err
	}
	fit := cfg.bankFit()
	banks, errs, chosen, met := fit.Fit(ir, irRate)
	if chosen == 0 {
		return errors.New("the fit stopped early")
	}
	bank := banks[chosen-1]
	if !cfg.pop {
		bank.NormalisePeak(fit.Grid(), fit.Rate)
	}
	newTarget := func() *analysis.FormantTarget { return fit.Target(ir, irRate) }

	fmt.Fprintf(w, "## %s\n\n%s Grid: %g Hz to %g Hz, %d points per octave, smoothed over %s, %s weights, soft floor %g dB below %s. The bank runs at %d Hz.\n\n",
		cfg.name, source, cfg.lo, cfg.hi, gridPerOctave, cfg.smoothing(), cfg.weightText(), cfg.floor, cfg.floorText(), cfg.rate)
	bands := thirdOctaves(cfg.lo, cfg.hi)
	irBands := analysis.IRResponse(ir, irRate, bands)
	base := make([]float64, len(bands))
	if cfg.harmonic() {
		base = bandBase(bands, float64(irRate))
	}
	fmt.Fprintf(w, "| Sections | Weighted error (dB) | Third-octave RMS 50-2000 Hz (dB) | Third-octave max 50-%g Hz (dB) | Multiply-adds per sample |\n| --- | --- | --- | --- | --- |\n", cfg.hi)
	for n := cfg.minN; n <= cfg.maxN; n++ {
		b := banks[n-1]
		r, mx := bandErrors(bands, irBands, base, b.BandsDB(bands, float64(cfg.rate)), cfg.floor, cfg.hi)
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

	responseTable(w, &cfg, bands, irBands, base, &bank, paths, &exhaust)
	if len(paths) > 0 {
		stageTable(w, bands, paths, &exhaust, cfg.rate, cfg.smooth, newTarget)
	}
	if cfg.out != "" {
		delayMS := 0.0
		if cfg.pop {
			delayMS = analysis.LegacyPopDelayMS(&paths[0], &exhaust)
		}
		if err := writeFragment(cfg.out, &cfg, &bank, errs[chosen-1], len(paths), delayMS); err != nil {
			return err
		}
		fmt.Fprintf(w, "Wrote `%s`.\n\n", cfg.out)
	}
	return nil
}

// bankFit returns the fit settings of the flags.
func (c *config) bankFit() analysis.BankFit {
	f := analysis.BankFit{
		Lo: c.lo, Hi: c.hi, PerOctave: gridPerOctave, Smooth: c.smooth, FloorDB: c.floor,
		EqualWeights: c.weight == "octave",
		MinN:         c.minN, MaxN: c.maxN, Tolerance: c.tol, Rate: float64(c.rate),
	}
	if c.harmonic() {
		f.HarmonicHz = inputLowPassHz
	}
	return f
}

// loadTarget returns the response to fit, its sample rate and a sentence
// that names its source. With --pop it is the pop stages of the first
// path, and every path must share them.
func loadTarget(cfg *config, paths []analysis.LegacyPath, e *analysis.LegacyExhaust) ([]float64, int, string, error) {
	if !cfg.pop {
		audio, err := wav.ReadFile(cfg.ir)
		if err != nil {
			return nil, 0, "", err
		}
		ir := analysis.IRTrim(audio.Mono(), engineSimTaps)
		if len(ir) == 0 {
			return nil, 0, "", fmt.Errorf("%s: no sample above the engine-sim threshold", cfg.ir)
		}
		return ir, audio.SampleRate, fmt.Sprintf("Impulse response `%s`: %d taps at %d Hz as engine-sim loads it.", cfg.ir, len(ir), audio.SampleRate), nil
	}
	h, err := analysis.LegacyPopImpulse(paths, e, float64(cfg.rate), pathTaps)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%s: %w", cfg.profile, err)
	}
	return h, cfg.rate, fmt.Sprintf("Pop stages of `%s` (wall loss, low shelf, chamber, mode cutoff and outlet of path 1): %d taps at %d Hz.", cfg.profile, len(h), cfg.rate), nil
}

// kindName returns the TOML name of a section kind.
func kindName(k string) string {
	if k == analysis.KindPeak {
		return "peak"
	}
	return k
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

// bandErrors returns the RMS of the band differences from 50 Hz to 2 kHz
// and the largest band difference from 50 Hz to hi between the response m
// and the reference ref, after the offset that matches their means from
// 50 Hz to 2 kHz. The largest difference leaves out bands whose level
// (ref plus base) is below the floor.
func bandErrors(bands [][2]float64, ref, base, m []float64, floor, hi float64) (rmsDB, maxDB float64) {
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
		if c >= 50 && c <= hi*1.01 && (floor >= 0 || ref[i]+base[i]-peak >= floor) {
			maxDB = math.Max(maxDB, math.Abs(d))
		}
	}
	return math.Sqrt(s / float64(max(n, 1))), maxDB
}

// responseTable writes the target, with the fitted bank in place of it,
// and with each legacy path in place of it, per band, relative to the
// highest band of the target. With --base harmonic, each column includes
// the 1.9 kHz input low pass and the derivative.
func responseTable(w io.Writer, cfg *config, bands [][2]float64, irBands, base []float64, bank *analysis.FormantBank, paths []analysis.LegacyPath, e *analysis.LegacyExhaust) {
	cols := [][]float64{irBands, bank.BandsDB(bands, float64(cfg.rate))}
	names := []string{"Target", "Fitted bank"}
	if !cfg.pop {
		names[0] = "engine-sim"
		for i := range paths {
			h := analysis.LegacyPathImpulse(&paths[i], e, float64(cfg.rate), pathTaps, analysis.LegacyAllStages())
			cols = append(cols, analysis.IRResponse(h, cfg.rate, bands))
			names = append(names, fmt.Sprintf("snarl path %d", i+1))
		}
	}
	// Each column moves so that its mean from 50 Hz to 2 kHz matches the
	// target, then all are relative to the highest target band.
	peak := math.Inf(-1)
	for i := range bands {
		peak = math.Max(peak, irBands[i]+base[i])
	}
	if cfg.harmonic() {
		fmt.Fprint(w, "Harmonic path (the 1.9 kHz input low pass, the derivative and the named response)")
	} else {
		fmt.Fprint(w, "Response")
	}
	fmt.Fprint(w, ", in dB relative to the highest target band. Each column after the first has the offset that matches its mean from 50 Hz to 2 kHz to the target.\n\n| Band (Hz) |")
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
		r, _ := bandErrors(bands, irBands, base, c, 0, cfg.hi)
		fmt.Fprintf(w, " %.2f |", r)
	}
	fmt.Fprint(w, "\n\n")
}

// stageTable writes the response of each linear stage of each path alone,
// per band relative to its own mean from 50 Hz to 2 kHz, its cost, and
// the weighted error of the whole path against the fit target.
func stageTable(w io.Writer, bands [][2]float64, paths []analysis.LegacyPath, e *analysis.LegacyExhaust, rate int, smooth float64, newTarget func() *analysis.FormantTarget) {
	for i := range paths {
		fmt.Fprintf(w, "Legacy path %d, each linear stage alone, in dB relative to its mean from 50 Hz to 2 kHz.\n\n| Band (Hz) |", i+1)
		var cols [][]float64
		for s := range analysis.LegacyStageCount {
			fmt.Fprintf(w, " %s |", analysis.LegacyStageNames[s])
			h := analysis.LegacyPathImpulse(&paths[i], e, float64(rate), pathTaps, analysis.LegacyOnlyStage(s))
			cols = append(cols, analysis.IRResponse(h, rate, bands))
		}
		fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+int(analysis.LegacyStageCount))+"\n")
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
		for s := range analysis.LegacyStageCount {
			fmt.Fprintf(w, " %d |", analysis.LegacyStageCost[s])
			total += analysis.LegacyStageCost[s]
		}
		t := newTarget()
		h := analysis.LegacyPathImpulse(&paths[i], e, float64(rate), pathTaps, analysis.LegacyAllStages())
		pe, _ := analysis.ShapeError(t, analysis.SmoothedResponse(h, rate, t.Hz, smooth))
		fmt.Fprintf(w, "\n\nThe linear stages of path %d cost %d multiply-adds per sample. The whole path is %.2f dB from the fit target (weighted error, best gain).\n\n", i+1, total, pe)
	}
}

// writeFragment writes the proposed TOML fragment for the bank: the
// formant bank of an impulse response, or with --pop the pop bank and the
// pop delay in milliseconds, the delay of the expansion chamber, which a
// fit of the magnitude leaves out.
func writeFragment(path string, cfg *config, b *analysis.FormantBank, errDB float64, paths int, delayMS float64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var s strings.Builder
	key, table := "formant_gain_db", "formant"
	if cfg.pop {
		key, table = "pop_formant_gain_db", "pop_formant"
		fmt.Fprintf(&s, "# Pop bank for the %s profile, written by tools/fit --pop.\n", cfg.name)
		fmt.Fprintf(&s, "# A cascade of %d biquads and one gain, fitted to the magnitude of the\n", len(b.Formants))
		fmt.Fprintf(&s, "# pop stages of %s at a73b874 (wall loss, low shelf, expansion\n", filepath.Base(cfg.profile))
		fmt.Fprintf(&s, "# chamber, mode cutoff and outlet). The gain is the fitted gain.\n")
	} else {
		fmt.Fprintf(&s, "# Formant bank for the %s profile, written by tools/fit.\n", cfg.name)
		fmt.Fprintf(&s, "# A cascade of %d biquads (Audio EQ Cookbook peaking, low pass and high\n# pass sections) and one gain, fitted\n", len(b.Formants))
		fmt.Fprintf(&s, "# to the magnitude of the engine-sim impulse response %s.\n", filepath.Base(cfg.ir))
		fmt.Fprintf(&s, "# The values are snarl's own fit parameters, not taps of the response.\n")
		fmt.Fprintf(&s, "# The gain sets the highest response of the bank on the grid to 0 dB.\n")
	}
	fmt.Fprintf(&s, "# Weighted error %.2f dB from %g Hz to %g Hz (smoothed over %s, %s\n# weights, soft floor", errDB, cfg.lo, cfg.hi, cfg.smoothing(), cfg.weightText())
	fmt.Fprintf(&s, " %g dB below %s).\n", cfg.floor, cfg.floorText())
	if paths > 0 && !cfg.pop {
		fmt.Fprintf(&s, "# engine-sim uses one impulse response for every exhaust system, so one\n")
		fmt.Fprintf(&s, "# bank serves every path of the profile (%d).\n", paths)
	}
	fmt.Fprintf(&s, "\n[exhaust]\n%s = %.2f\n", key, b.GainDB)
	if cfg.pop {
		fmt.Fprintf(&s, "pop_delay_ms = %.3f\n", delayMS)
	}
	for _, f := range b.Formants {
		fmt.Fprintf(&s, "\n[[%s]]\nkind = %q\nhz = %.1f\nq = %.3f\n", table, kindName(f.Kind), f.Hz, f.Q)
		if f.Kind == analysis.KindPeak {
			fmt.Fprintf(&s, "gain_db = %.2f\n", f.GainDB)
		}
	}
	return os.WriteFile(path, []byte(s.String()), 0o644)
}
