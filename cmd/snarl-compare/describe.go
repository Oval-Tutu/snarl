package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// describe writes Markdown tables per file that describe each window of
// the scenario on its own: the level, the spectral shape and the band
// balance, then the whole engine orders relative to the power of the
// window.
func describe(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || len(paths) == 0 {
		return errors.New("--describe needs --scenario and at least one WAV file")
	}
	if cfg.maxOrder < 1 || cfg.window <= 0 || cfg.settle < 0 {
		return errors.New("--orders must be at least 1, --window positive and --settle not negative")
	}
	sc, err := scenario.ParseFile(cfg.scenario)
	if err != nil {
		return err
	}
	orders := analysis.Orders(cfg.maxOrder)
	for _, path := range paths {
		audio, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		x, rate := audio.Mono(), audio.SampleRate
		windows := analysis.Windows(sc, float64(len(x))/float64(rate), analysis.Options{Settle: cfg.settle, Window: cfg.window})
		ds := analysis.Describe(x, rate, windows, orders)
		fmt.Fprintf(w, "## %s\n\n", path)
		if len(ds) == 0 {
			fmt.Fprint(w, "No window overlaps the audio.\n\n")
			continue
		}
		describeTable(w, ds)
		orderTable(w, ds, orders)
	}
	return nil
}

// windowRPM returns the mean RPM of a window.
func windowRPM(d *analysis.Description) float64 {
	return (d.RPMStart + d.RPMEnd) / 2
}

func describeTable(w io.Writer, ds []analysis.Description) {
	fmt.Fprint(w, "| Window | Throttle | RPM | Loudness (LUFS) | RMS (dBFS) | Peak (dBFS) | Crest (dB) | Centroid (Hz) | Flatness | Rolloff (Hz) |")
	for _, b := range analysis.DescribeBands {
		fmt.Fprintf(w, " %s (dB) |", bandName(analysis.ResidualBand{Lo: b[0], Hi: b[1]}))
	}
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 10+len(analysis.DescribeBands))+"\n")
	for i := range ds {
		d := &ds[i]
		fmt.Fprintf(w, "| %s | %.2f | %.0f | %.1f | %.1f | %.1f | %.1f | %.0f | %.4f | %.0f |",
			d.Label, d.Throttle, windowRPM(d), d.LoudnessLUFS, d.RMSDBFS, d.PeakDBFS, d.CrestDB, d.CentroidHz, d.Flatness, d.RolloffHz)
		for _, v := range d.Bands {
			fmt.Fprintf(w, " %.1f |", v)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, "\nLoudness: the ungated K-weighted mean square of the window. Centroid, flatness and rolloff (85 per cent): means over Hann frames of 4096 samples with half overlap, as ffmpeg aspectralstats. Bands: the octave band power relative to the power of the window.\n\n")
}

// orderTable writes the power of each whole order relative to the power of
// the window, the sum of the whole orders and the sum of the half orders,
// all in dB. A sinusoid of amplitude A has the power A^2/2.
func orderTable(w io.Writer, ds []analysis.Description, orders []float64) {
	var whole, half []int
	for i, o := range orders {
		if o == math.Trunc(o) {
			whole = append(whole, i)
		} else {
			half = append(half, i)
		}
	}
	fmt.Fprint(w, "| Window | RPM |")
	for _, i := range whole {
		fmt.Fprintf(w, " %g |", orders[i])
	}
	fmt.Fprint(w, " Whole sum | Half sum |\n|"+strings.Repeat(" --- |", 4+len(whole))+"\n")
	rows := 0
	for i := range ds {
		d := &ds[i]
		if d.Orders == nil {
			continue
		}
		rel := func(j int) float64 { return d.Orders[j] - 3.0103 - d.RMSDBFS }
		sum := func(idx []int) float64 {
			var p float64
			for _, j := range idx {
				p += math.Pow(10, rel(j)/10)
			}
			if p <= 0 {
				return -120
			}
			return 10 * math.Log10(p)
		}
		fmt.Fprintf(w, "| %s | %.0f |", d.Label, windowRPM(d))
		for _, j := range whole {
			fmt.Fprintf(w, " %.1f |", rel(j))
		}
		fmt.Fprintf(w, " %.1f | %.1f |\n", sum(whole), sum(half))
		rows++
	}
	if rows == 0 {
		fmt.Fprint(w, "\nNo window has an order spectrum.\n")
	}
	fmt.Fprint(w, "\nOrders: the power of each whole engine order relative to the power of the window, in dB. Whole sum and half sum: the summed power of the whole orders and of the half orders up to the highest order, relative to the power of the window.\n\n")
}
