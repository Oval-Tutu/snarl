package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// residuals writes Markdown tables per file with the residual of each hold
// of the scenario: the RPM that fits the cycle period best, the number of
// whole cycles, the signal level, the residual relative to the signal and
// the residual in each band relative to the signal. Each hold starts after
// the settle time.
func residuals(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || len(paths) == 0 {
		return errors.New("--residual needs --scenario and at least one WAV file")
	}
	if cfg.settle < 0 {
		return errors.New("--settle must not be negative")
	}
	sc, err := scenario.ParseFile(cfg.scenario)
	if err != nil {
		return err
	}
	for _, path := range paths {
		audio, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		x, rate := audio.Mono(), audio.SampleRate
		windows := analysis.Windows(sc, float64(len(x))/float64(rate), analysis.Options{Settle: cfg.settle, Window: 1})
		fmt.Fprintf(w, "## %s\n\n", path)
		var labels []string
		var rs []analysis.ResidualResult
		for _, win := range windows {
			if win.Kind != "hold" {
				continue
			}
			i0, i1 := int(win.Start*float64(rate)), min(int(win.End*float64(rate)), len(x))
			r, ok := analysis.Residual(x[i0:i1], rate, win.RPMStart)
			if !ok {
				fmt.Fprintf(w, "%s: too short or silent, no residual.\n\n", win.Label)
				continue
			}
			label := fmt.Sprintf("%s | %.2f", win.Label, win.Throttle)
			if r.AtEdge {
				label = fmt.Sprintf("%s (period at search edge) | %.2f", win.Label, win.Throttle)
			}
			labels, rs = append(labels, label), append(rs, r)
		}
		if len(rs) > 0 {
			residualTable(w, labels, rs)
		}
	}
	return nil
}

// residualTable writes the table of the residuals rs of the holds with the
// labels, with the band residuals relative to the whole signal, then a
// table of the band residuals relative to the signal in the same band.
func residualTable(w io.Writer, labels []string, rs []analysis.ResidualResult) {
	bands := func() {
		for _, b := range rs[0].Bands {
			fmt.Fprintf(w, " %s (dB) |", bandName(b))
		}
	}
	fmt.Fprint(w, "| Hold | Throttle | RPM | Cycles | Signal (dBFS) | Residual (dB) |")
	bands()
	fmt.Fprint(w, " AM peak (dB) | AM trough (dB) |")
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 8+len(rs[0].Bands))+"\n")
	for i, r := range rs {
		fmt.Fprintf(w, "| %s | %.1f | %d | %.1f | %.1f |", labels[i], r.RPM, r.Cycles, r.SignalDBFS, r.ResidualDB)
		for _, b := range r.Bands {
			fmt.Fprintf(w, " %.1f |", b.DB)
		}
		fmt.Fprintf(w, " %.1f | %.1f |\n", r.AMPeakDB, r.AMTroughDB)
	}
	fmt.Fprint(w, "\nAM peak and AM trough: the highest and the lowest residual power of 96 crank angle sectors of the cycle, relative to the mean sector power.\n")
	fmt.Fprint(w, "\nThe residual in each band relative to the signal in the same band:\n\n| Hold | Throttle |")
	bands()
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 2+len(rs[0].Bands))+"\n")
	for i, r := range rs {
		fmt.Fprintf(w, "| %s |", labels[i])
		for _, b := range r.Bands {
			fmt.Fprintf(w, " %.1f |", b.InBandDB)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

// bandName returns the band edges in hertz or kilohertz, for example 500-2k.
func bandName(b analysis.ResidualBand) string {
	f := func(v float64) string {
		if v >= 1000 {
			return fmt.Sprintf("%gk", v/1000)
		}
		return fmt.Sprintf("%g", v)
	}
	return f(b.Lo) + "-" + f(b.Hi)
}
