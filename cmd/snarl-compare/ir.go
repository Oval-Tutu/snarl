package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// irFitLo and irFitHi are the edges in hertz of the range of the RMS
	// response differences, where the engine-sim output carries harmonic
	// content.
	irFitLo, irFitHi = 50, 2000
	// irWideHi is the upper edge in hertz of the wide range of the RMS
	// response differences.
	irWideHi = 10000
	// inputLowPassHz is the cut-off of the fourth-order Butterworth low
	// pass on each synthesiser input channel of engine-sim.
	inputLowPassHz = 1900
)

// irFIRTaps and irPoles are the lengths of the shorter FIR filters and the
// orders of the all-pole filters that the IR table compares with the
// engine-sim impulse response.
var (
	irFIRTaps = []int{4096, 2048, 1024, 512, 256, 128}
	irPoles   = []int{8, 16, 32, 64}
)

// irs writes a Markdown table per impulse response WAV file: its length as
// engine-sim loads it, its energy decay, and its third-octave response
// next to the responses of shorter FIR filters and of all-pole fits, with
// the RMS differences from the engine-sim response.
func irs(w io.Writer, paths []string) error {
	if len(paths) == 0 {
		return errors.New("--ir needs at least one impulse response WAV file")
	}
	for _, path := range paths {
		audio, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		raw, rate := audio.Mono(), audio.SampleRate
		ir := analysis.IRTrim(raw, analysis.EngineSimTaps)
		if len(ir) == 0 {
			return fmt.Errorf("%s: no sample above the engine-sim threshold", path)
		}
		t20, t40 := analysis.EnergyDecay(ir, rate)
		fmt.Fprintf(w, "## %s\n\n%d samples at %d Hz, %d taps as engine-sim loads it (%.1f ms). The energy still to come falls 20 dB below the total after %.1f ms and 40 dB after %.1f ms.\n\n",
			path, len(raw), rate, len(ir), 1000*float64(len(ir))/float64(rate), 1000*t20, 1000*t40)
		bands := analysis.ThirdOctaveBands(rate)
		names := []string{"Engine-sim"}
		rs := [][]float64{analysis.IRResponse(ir, rate, bands)}
		for _, n := range irFIRTaps {
			if n < len(ir) {
				names = append(names, fmt.Sprintf("FIR %d", n))
				rs = append(rs, analysis.IRResponse(ir[:n], rate, bands))
			}
		}
		for _, p := range irPoles {
			if a, g, ok := analysis.AllPole(ir, p); ok {
				names = append(names, fmt.Sprintf("All-pole %d", p))
				rs = append(rs, analysis.AllPoleResponse(a, g, rate, bands))
			}
		}
		irTable(w, bands, names, rs)
		path := analysis.SynthPathResponse(ir, rate, inputLowPassHz, bands)
		peak := math.Inf(-1)
		for _, v := range path {
			peak = max(peak, v)
		}
		fmt.Fprint(w, "The harmonic path of the synthesiser: the 1.9 kHz input low pass, the derivative and this impulse response, relative to its highest band, in dB.\n\n| Band (Hz) | Harmonic path |\n| --- | --- |\n")
		for i, b := range bands {
			fmt.Fprintf(w, "| %.0f | %.1f |\n", math.Sqrt(b[0]*b[1]), path[i]-peak)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// irTable writes the responses rs relative to the highest band of the
// first response, one column per response, and the RMS differences from
// the first response over the fit range and the wide range.
func irTable(w io.Writer, bands [][2]float64, names []string, rs [][]float64) {
	ref := rs[0]
	peak := math.Inf(-1)
	for _, v := range ref {
		peak = max(peak, v)
	}
	fmt.Fprint(w, "| Band (Hz) |")
	for _, n := range names {
		fmt.Fprintf(w, " %s |", n)
	}
	fmt.Fprint(w, "\n|"+strings.Repeat(" --- |", 1+len(names))+"\n")
	for i, b := range bands {
		fmt.Fprintf(w, "| %.0f |", math.Sqrt(b[0]*b[1]))
		for _, r := range rs {
			fmt.Fprintf(w, " %.1f |", r[i]-peak)
		}
		fmt.Fprintln(w)
	}
	for _, hi := range []float64{irFitHi, irWideHi} {
		fmt.Fprintf(w, "| RMS difference %g-%g Hz |", float64(irFitLo), hi)
		for _, r := range rs {
			var sum float64
			n := 0
			for i, b := range bands {
				c := math.Sqrt(b[0] * b[1])
				if c >= irFitLo && c <= hi {
					d := r[i] - ref[i]
					sum += d * d
					n++
				}
			}
			fmt.Fprintf(w, " %.2f |", math.Sqrt(sum/float64(max(n, 1))))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprint(w, "\nLevels: the power response in each third-octave band relative to the highest band of the engine-sim response, in dB. FIR N: the first N taps. All-pole P: the linear prediction fit of order P (P/2 biquads). RMS difference: the root mean square of the band differences from the engine-sim response, over the bands whose centre is in the range.\n\n")
}
