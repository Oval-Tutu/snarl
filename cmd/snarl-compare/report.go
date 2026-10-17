package main

import (
	"fmt"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

var metricTitles = map[string]string{
	"order_distance_db":    "Order spectrum distance (dB)",
	"third_octave_lsd_db":  "Third-octave log-spectral distance (dB)",
	"mrstft_total":         "Multi-resolution STFT distance",
	"crest_factor_diff_db": "Crest factor difference (dB)",
	"spectral_flux_diff":   "Spectral flux difference",
}

func markdown(res *Result) string {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	p("# snarl-compare: %s vs %s\n\n", stem(res.A.Path), stem(res.B.Path))
	if res.Scenario != "" {
		p("Scenario: `%s`.\n\n", res.Scenario)
	}
	p("Both inputs are downmixed to mono and normalised to %.1f LUFS (ITU-R BS.1770-4 integrated loudness) before analysis at %d Hz. A is the reference for spectral convergence.\n\n", res.TargetLUFS, res.SampleRate)
	p("| Input | File | Rate (Hz) | Channels | Duration (s) | Loudness (LUFS) | Gain (dB) | Spectrogram |\n")
	p("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, in := range []struct {
		name string
		in   Input
	}{{"A", res.A}, {"B", res.B}} {
		spec := "none"
		if in.in.Spectrogram != "" {
			spec = fmt.Sprintf("[%s](%s)", in.in.Spectrogram, in.in.Spectrogram)
		}
		p("| %s | `%s` | %d | %d | %.3f | %.2f | %+.2f | %s |\n", in.name, in.in.Path, in.in.SampleRate, in.in.Channels, in.in.Duration, in.in.LoudnessDB, in.in.GainDB, spec)
	}

	p("\n## Worst window per metric\n\n")
	p("| Metric | Window | Value |\n| --- | --- | --- |\n")
	for _, w := range res.Worst {
		p("| %s | %s | %.3f |\n", metricTitles[w.Metric], w.Label, w.Value)
	}

	p("\n## Windows\n")
	for i := range res.Windows {
		writeWindow(&b, &res.Windows[i])
	}

	if len(res.Notes) > 0 {
		p("\n## Notes\n\n")
		for _, n := range res.Notes {
			p("- %s\n", n)
		}
	}
	p("\n## Metrics\n\n")
	p("- Order spectrum: the audio is resampled at constant crank-angle steps from the scenario RPM, then each order from 0.5 to %g is measured in dBFS. The distance is the root mean square of the level differences.\n", res.Orders[len(res.Orders)-1])
	p("- Third-octave log-spectral distance: the root mean square difference of the long-term band levels from 25 Hz to 16 kHz.\n")
	p("- Multi-resolution STFT distance: the mean over FFT sizes %v of spectral convergence plus the mean absolute natural-log magnitude difference. Hann window, 75 per cent overlap.\n", res.FFTSizes)
	p("- Crest factor: the peak to RMS ratio. Spectral flux: the mean distance between successive unit-norm magnitude spectra (FFT 2048, hop 512).\n")
	return b.String()
}

func writeWindow(b *strings.Builder, m *analysis.WindowMetrics) {
	p := func(format string, args ...any) { fmt.Fprintf(b, format, args...) }
	p("\n### %s\n\n", m.Label)
	p("%.3f s to %.3f s, throttle %.2f.\n\n", m.Start, m.End, m.Throttle)
	p("| Metric | A | B | Distance |\n| --- | --- | --- | --- |\n")
	if m.Order != nil {
		p("| Dominant order | %g | %g | |\n", m.Order.DominantA, m.Order.DominantB)
		p("| %s | | | %.3f |\n", metricTitles["order_distance_db"], m.Order.Distance)
	}
	p("| %s | | | %.3f |\n", metricTitles["third_octave_lsd_db"], m.ThirdOctaveLSD)
	p("| %s | | | %.3f |\n", metricTitles["mrstft_total"], m.MRSTFT.Total)
	p("| Spectral convergence | | | %.3f |\n", m.MRSTFT.SpectralConvergence)
	p("| Log magnitude distance | | | %.3f |\n", m.MRSTFT.LogMagnitude)
	p("| Crest factor (dB) | %.2f | %.2f | %.2f |\n", m.CrestFactorDB.A, m.CrestFactorDB.B, m.CrestFactorDB.Diff)
	p("| Spectral flux | %.4f | %.4f | %.4f |\n", m.SpectralFlux.A, m.SpectralFlux.B, m.SpectralFlux.Diff)
}
