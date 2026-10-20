// Command snarl-compare compares two engine renders that the same scenario
// drives, and writes report.md, metrics.json and optional spectrograms.
// With --residual, it measures instead the residual of each hold of one or
// more renders: the part that does not repeat from one engine cycle to the
// next.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// Input describes one compared file.
type Input struct {
	Path        string  `json:"path"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	Duration    float64 `json:"duration_s"`
	LoudnessDB  float64 `json:"integrated_lufs"`
	GainDB      float64 `json:"gain_db"`
	Spectrogram string  `json:"spectrogram,omitempty"`
}

// Result is the content of metrics.json.
type Result struct {
	A          Input                    `json:"a"`
	B          Input                    `json:"b"`
	Scenario   string                   `json:"scenario,omitempty"`
	TargetLUFS float64                  `json:"target_lufs"`
	SampleRate int                      `json:"analysis_sample_rate"`
	Orders     []float64                `json:"orders"`
	FFTSizes   []int                    `json:"fft_sizes"`
	LowBandHz  [2]float64               `json:"low_band_hz"`
	LowBand    analysis.LowBandSummary  `json:"low_band_median"`
	Windows    []analysis.WindowMetrics `json:"windows"`
	Worst      []analysis.Worst         `json:"worst"`
	Notes      []string                 `json:"notes,omitempty"`
}

type config struct {
	scenario     string
	out          string
	maxOrder     float64
	window       float64
	settle       float64
	fftSizes     string
	target       float64
	spectrograms bool
	band         string
	residual     bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "snarl-compare:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var cfg config
	fs := flag.NewFlagSet("snarl-compare", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: snarl-compare [flags] A.wav B.wav")
		fmt.Fprintln(fs.Output(), "       snarl-compare --residual --scenario S.scn [flags] A.wav [B.wav ...]")
		fs.PrintDefaults()
	}
	fs.StringVar(&cfg.scenario, "scenario", "", "scenario file that drove both renders (required for the order analysis)")
	fs.StringVar(&cfg.out, "out", "", "output directory (default out/compare/<A>-vs-<B>)")
	fs.Float64Var(&cfg.maxOrder, "orders", 20, "highest engine order; orders run from 0.5 in steps of 0.5")
	fs.Float64Var(&cfg.window, "window", 0.5, "length in seconds of each window along a sweep")
	fs.Float64Var(&cfg.settle, "settle", 0.25, "seconds to skip at the start of each segment")
	fs.StringVar(&cfg.fftSizes, "fft-sizes", "4096,2048,1024,512,256,128,64", "comma-separated multi-resolution STFT sizes, each a power of two")
	fs.Float64Var(&cfg.target, "target", -23, "integrated loudness in LUFS that both inputs are normalised to")
	fs.StringVar(&cfg.band, "band", fmt.Sprintf("%d:%d", analysis.DefaultBandLo, analysis.DefaultBandHi), "low band LO:HI in hertz for the low-band metrics")
	fs.BoolVar(&cfg.spectrograms, "spectrograms", true, "write spectrogram PNGs with ffmpeg when it is on PATH")
	fs.BoolVar(&cfg.residual, "residual", false, "print the residual of each hold of each file to standard output, as Markdown tables, instead of the comparison")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.residual {
		return residuals(os.Stdout, fs.Args(), &cfg)
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return errors.New("need two WAV files")
	}
	sizes, err := parseSizes(cfg.fftSizes)
	if err != nil {
		return err
	}
	lo, hi, err := parseBand(cfg.band)
	if err != nil {
		return err
	}
	if cfg.maxOrder < 0.5 || cfg.window <= 0 || cfg.settle < 0 {
		return errors.New("--orders must be at least 0.5, --window positive and --settle not negative")
	}
	pathA, pathB := fs.Arg(0), fs.Arg(1)
	if cfg.out == "" {
		cfg.out = filepath.Join("out", "compare", stem(pathA)+"-vs-"+stem(pathB))
	}
	opts := analysis.Options{
		Orders:   analysis.Orders(cfg.maxOrder),
		Window:   cfg.window,
		Settle:   cfg.settle,
		FFTSizes: sizes,
		BandLo:   lo,
		BandHi:   hi,
	}
	res, err := compare(pathA, pathB, &cfg, opts)
	if err != nil {
		return err
	}
	return write(res, &cfg)
}

func compare(pathA, pathB string, cfg *config, opts analysis.Options) (*Result, error) {
	res := &Result{
		Scenario:   cfg.scenario,
		TargetLUFS: cfg.target,
		Orders:     opts.Orders,
		FFTSizes:   opts.FFTSizes,
		LowBandHz:  [2]float64{opts.BandLo, opts.BandHi},
	}
	audioA, err := wav.ReadFile(pathA)
	if err != nil {
		return nil, err
	}
	audioB, err := wav.ReadFile(pathB)
	if err != nil {
		return nil, err
	}
	a, b := audioA.Mono(), audioB.Mono()
	rate := audioA.SampleRate
	res.SampleRate = rate
	if audioB.SampleRate != rate {
		b = analysis.Resample(b, audioB.SampleRate, rate)
		res.Notes = append(res.Notes, fmt.Sprintf("B was resampled from %d Hz to %d Hz.", audioB.SampleRate, rate))
	}
	res.A, err = normalise(a, rate, pathA, audioA, cfg.target)
	if err != nil {
		return nil, err
	}
	res.B, err = normalise(b, rate, pathB, audioB, cfg.target)
	if err != nil {
		return nil, err
	}
	duration := float64(min(len(a), len(b))) / float64(rate)
	if math.Abs(res.A.Duration-res.B.Duration) > 0.01 {
		res.Notes = append(res.Notes, fmt.Sprintf("The inputs differ in length. The comparison uses the first %.3f s.", duration))
	}
	var sc *scenario.Scenario
	if cfg.scenario != "" {
		sc, err = scenario.ParseFile(cfg.scenario)
		if err != nil {
			return nil, err
		}
		if d := sc.Duration(); math.Abs(d-duration) > 0.05 {
			res.Notes = append(res.Notes, fmt.Sprintf("The scenario lasts %.3f s and the audio %.3f s.", d, duration))
		}
	} else {
		res.Notes = append(res.Notes, "No --scenario was given, so the order analysis was skipped and the whole file is one window.")
	}
	windows := analysis.Windows(sc, duration, opts)
	if len(windows) == 0 {
		return nil, errors.New("no scenario segment overlaps the audio")
	}
	res.Windows = analysis.Compare(a, b, rate, windows, opts)
	res.Worst = analysis.WorstWindows(res.Windows)
	res.LowBand = analysis.SummariseLowBand(res.Windows)
	return res, nil
}

// normalise measures the mono downmix x of audio and scales x in place to
// the target loudness.
func normalise(x []float64, rate int, path string, audio *wav.Audio, target float64) (Input, error) {
	in := Input{
		Path:       path,
		SampleRate: audio.SampleRate,
		Channels:   len(audio.Channels),
		Duration:   audio.Duration(),
	}
	in.LoudnessDB = analysis.IntegratedLoudness([][]float64{x}, rate)
	if math.IsInf(in.LoudnessDB, -1) {
		return in, fmt.Errorf("%s: silent or shorter than 0.4 s, so it has no integrated loudness", path)
	}
	in.GainDB = target - in.LoudnessDB
	analysis.Scale(x, in.GainDB)
	return in, nil
}

func write(res *Result, cfg *config) error {
	if err := os.MkdirAll(cfg.out, 0o750); err != nil {
		return err
	}
	if cfg.spectrograms {
		spectrograms(res, cfg.out)
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.out, "metrics.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.out, "report.md"), []byte(markdown(res)), 0o644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", filepath.Join(cfg.out, "report.md"))
	return nil
}

// spectrograms writes one PNG per input with ffmpeg showspectrumpic. It adds
// a note instead when ffmpeg is not on PATH or fails.
func spectrograms(res *Result, dir string) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		res.Notes = append(res.Notes, "ffmpeg is not on PATH, so no spectrograms were written.")
		fmt.Fprintln(os.Stderr, "snarl-compare: ffmpeg is not on PATH, skipping spectrograms")
		return
	}
	for _, in := range []*Input{&res.A, &res.B} {
		name := "spectrogram-a.png"
		if in == &res.B {
			name = "spectrogram-b.png"
		}
		cmd := exec.CommandContext(context.Background(), ffmpeg, //nolint:gosec // ffmpeg from PATH with fixed options and user-supplied paths.
			"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", in.Path,
			"-lavfi", "showspectrumpic=s=1280x480:scale=log:fscale=log", filepath.Join(dir, name))
		if out, err := cmd.CombinedOutput(); err != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("ffmpeg failed on %s: %v %s", in.Path, err, strings.TrimSpace(string(out))))
			continue
		}
		in.Spectrogram = name
	}
}

func parseSizes(s string) ([]int, error) {
	var sizes []int
	for f := range strings.SplitSeq(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 4 || n&(n-1) != 0 {
			return nil, fmt.Errorf("--fft-sizes: %q is not a power of two from 4", f)
		}
		sizes = append(sizes, n)
	}
	return sizes, nil
}

func parseBand(s string) (lo, hi float64, err error) {
	l, h, found := strings.Cut(s, ":")
	if found {
		lo, err = strconv.ParseFloat(strings.TrimSpace(l), 64)
		if err == nil {
			hi, err = strconv.ParseFloat(strings.TrimSpace(h), 64)
		}
	}
	if !found || err != nil || lo <= 0 || hi <= lo {
		return 0, 0, fmt.Errorf("--band: %q is not LO:HI in hertz with 0 < LO < HI", s)
	}
	return lo, hi, nil
}

func stem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
