package main

import (
	"fmt"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
)

var metricTitles = map[string]string{
	"order_distance_db":          "Order spectrum distance (dB)",
	"third_octave_lsd_db":        "Third-octave log-spectral distance (dB)",
	"mrstft_total":               "Multi-resolution STFT distance",
	"crest_factor_diff_db":       "Crest factor difference (dB)",
	"spectral_flux_diff":         "Spectral flux difference",
	"low_band_order_distance_db": "Low-band order spectrum distance (dB)",
	"low_band_lsd_db":            "Low-band third-octave log-spectral distance (dB)",
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

	writeLowBandSummary(&b, res)

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
	p("- Low band: %g Hz to %g Hz. Tune against the low-band metrics. The full-spectrum metrics above describe the whole sound, and B is not the target above the low band.\n", res.LowBandHz[0], res.LowBandHz[1])
	p("- Low-band order spectrum distance: the order spectrum distance over the orders whose frequency at the mean window RPM is in the low band.\n")
	p("- Low-band third-octave log-spectral distance: the third-octave distance over the bands whose centre frequency is in the low band.\n")
	p("- Low-band balance: the power in the low band relative to the power of the whole spectrum above 0 Hz, in dB (Welch, FFT 8192). Higher means more of the sound is in the low band.\n")
	p("- Half orders: the summed power of the half orders (0.5, 1.5, 2.5, ...) relative to the summed power of the whole orders, both in the low band, in dB. Higher means more uneven firing or more cycle-rate content.\n")
	p("- Cycle variation: the input is band-passed to the low band (fourth-order Butterworth high-pass and low-pass, with 0.2 s of audio before the window to settle the filters), resampled at 1440 steps per engine cycle of 720 crank degrees from the scenario RPM, and split into whole cycles. Energy CV is the standard deviation of the per-cycle energy divided by its mean: 0 when every cycle is equally loud. Cycle correlation is the mean normalised correlation of each cycle with the next at zero lag: 1 when the waveform repeats exactly, lower when it changes from cycle to cycle. Higher energy CV and lower correlation mean more entropy in the rumble. Windows with fewer than three whole cycles have no cycle metrics.\n")
	p("- Low-band medians: the median of each low-band metric over the windows that have it.\n")
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
	l := m.LowBand
	if l == nil {
		return
	}
	if l.Order != nil {
		p("| %s | | | %.3f |\n", metricTitles["low_band_order_distance_db"], l.Order.Distance)
		if h := l.Order.HalfOrderDB; h != nil {
			p("| Low-band half orders (dB) | %.2f | %.2f | %.2f |\n", h.A, h.B, h.Diff)
		}
	}
	p("| %s | | | %.3f |\n", metricTitles["low_band_lsd_db"], l.ThirdOctaveLSD)
	p("| Low-band balance (dB) | %.2f | %.2f | %.2f |\n", l.BalanceDB.A, l.BalanceDB.B, l.BalanceDB.Diff)
	if c := l.Cycles; c != nil {
		p("| Low-band cycle energy CV | %.3f | %.3f | %.3f |\n", c.EnergyCV.A, c.EnergyCV.B, c.EnergyCV.Diff)
		p("| Low-band cycle correlation | %.3f | %.3f | %.3f |\n", c.Correlation.A, c.Correlation.B, c.Correlation.Diff)
	}
}

func writeLowBandSummary(b *strings.Builder, res *Result) {
	p := func(format string, args ...any) { fmt.Fprintf(b, format, args...) }
	s := res.LowBand
	p("\n## Low band, %g Hz to %g Hz (median over windows)\n\n", res.LowBandHz[0], res.LowBandHz[1])
	p("| Metric | A | B | Distance |\n| --- | --- | --- | --- |\n")
	if s.OrderDistance != nil {
		p("| %s | | | %.3f |\n", metricTitles["low_band_order_distance_db"], *s.OrderDistance)
	}
	if s.ThirdOctaveLSD != nil {
		p("| %s | | | %.3f |\n", metricTitles["low_band_lsd_db"], *s.ThirdOctaveLSD)
	}
	for _, row := range []struct {
		name   string
		format string
		v      *analysis.Pair
	}{
		{"Low-band balance (dB)", "%.2f", s.BalanceDB},
		{"Low-band half orders (dB)", "%.2f", s.HalfOrderDB},
		{"Low-band cycle energy CV", "%.3f", s.EnergyCV},
		{"Low-band cycle correlation", "%.3f", s.Correlation},
	} {
		if row.v != nil {
			f := row.format
			p("| %s | "+f+" | "+f+" | "+f+" |\n", row.name, row.v.A, row.v.B, row.v.Diff)
		}
	}
}
