package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// bursts writes a Markdown table per file with the burst measure of the
// residual of each hold of the scenario, one row per hold and band: the
// residual level in the band relative to the signal, the modulation depth
// of the band envelope, its cycle-locked part, the envelope crest factor and
// the spectral flatness. Each hold starts after the settle time.
func bursts(w io.Writer, paths []string, cfg *config) error {
	if cfg.scenario == "" || len(paths) == 0 {
		return errors.New("--burst needs --scenario and at least one WAV file")
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
		var rs []analysis.BurstResult
		for _, win := range windows {
			if win.Kind != "hold" {
				continue
			}
			i0, i1 := int(win.Start*float64(rate)), min(int(win.End*float64(rate)), len(x))
			r, ok := analysis.ResidualBurst(x[i0:i1], rate, win.RPMStart)
			if !ok {
				fmt.Fprintf(w, "%s: too short or silent, no burst measure.\n\n", win.Label)
				continue
			}
			label := fmt.Sprintf("%s | %.2f", win.Label, win.Throttle)
			if r.AtEdge {
				label = fmt.Sprintf("%s (period at search edge) | %.2f", win.Label, win.Throttle)
			}
			labels, rs = append(labels, label), append(rs, r)
		}
		if len(rs) > 0 {
			burstTable(w, labels, rs)
		}
	}
	return nil
}

// burstTable writes the table of the burst measures rs of the holds with the
// labels.
func burstTable(w io.Writer, labels []string, rs []analysis.BurstResult) {
	fmt.Fprint(w, "| Hold | Throttle | RPM | Band (Hz) | Level (dB) | Mod (dB) | Locked (dB) | Crest (dB) | Flatness |\n")
	fmt.Fprint(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for i, r := range rs {
		for _, b := range r.Bands {
			fmt.Fprintf(w, "| %s | %.1f | %s | %.1f | %.1f | %.1f | %.1f | %.3f |\n",
				labels[i], r.RPM, bandName(analysis.ResidualBand{Lo: b.Lo, Hi: b.Hi}), b.LevelDB, b.ModDB, b.LockedDB, b.CrestDB, b.Flatness)
		}
	}
	fmt.Fprint(w, "\nLevel: the residual in the band relative to the whole signal. Mod: the variance of the band envelope (5 ms frames, 96 per cycle) over its squared mean. Locked: the part of Mod that repeats with the engine cycle. Crest: the 99th percentile of the envelope over its mean. Flatness: the spectral flatness in the band, weighted by the frame power. A steady noise gives a Mod of about -7, -10 and -13 dB in the three bands, a Locked at the floor and a Crest of 2 to 4 dB.\n\n")
}
