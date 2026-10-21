package esynth

import (
	"fmt"
	"math"
)

// Difference is the sample-by-sample difference of a port output from the
// engine-sim output, in 16-bit steps.
type Difference struct {
	Samples   int
	Differing int
	// First is the index of the first differing sample, or -1. FirstPort
	// and FirstRef are its values.
	First               int
	FirstPort, FirstRef int16
	// PeakDiff and RMSDiff are the peak and root mean square of the
	// difference.
	PeakDiff, RMSDiff float64
	// PeakSignal and RMSSignal are the peak and root mean square of the
	// engine-sim output.
	PeakSignal, RMSSignal float64
}

// Compare returns the difference of port from ref, which must have the same
// length.
func Compare(port, ref []int16) (Difference, error) {
	if len(port) != len(ref) {
		return Difference{}, fmt.Errorf("the port output has %d samples, engine-sim %d", len(port), len(ref))
	}
	d := Difference{Samples: len(ref), First: -1}
	var sumDiff, sumSignal float64
	for i, r := range ref {
		e := float64(port[i]) - float64(r)
		if e != 0 {
			if d.Differing == 0 {
				d.First, d.FirstPort, d.FirstRef = i, port[i], r
			}
			d.Differing++
		}
		d.PeakDiff = max(d.PeakDiff, math.Abs(e))
		d.PeakSignal = max(d.PeakSignal, math.Abs(float64(r)))
		sumDiff += e * e
		sumSignal += float64(r) * float64(r)
	}
	if d.Samples > 0 {
		d.RMSDiff = math.Sqrt(sumDiff / float64(d.Samples))
		d.RMSSignal = math.Sqrt(sumSignal / float64(d.Samples))
	}
	return d, nil
}

// PeakDB returns the peak difference relative to the peak signal in dB. It
// is -Inf for identical outputs.
func (d Difference) PeakDB() float64 {
	return 20 * math.Log10(d.PeakDiff/d.PeakSignal)
}

// RMSDB returns the RMS difference relative to the RMS signal in dB. It is
// -Inf for identical outputs.
func (d Difference) RMSDB() float64 {
	return 20 * math.Log10(d.RMSDiff/d.RMSSignal)
}

// String formats the difference for a report.
func (d Difference) String() string {
	if d.Differing == 0 {
		return fmt.Sprintf("%d samples, identical", d.Samples)
	}
	return fmt.Sprintf("%d samples, %d differ from sample %d (port %d, engine-sim %d), peak difference %g (%.1f dB below the peak), RMS difference %.4g (%.1f dB below the RMS)",
		d.Samples, d.Differing, d.First, d.FirstPort, d.FirstRef, d.PeakDiff, -d.PeakDB(), d.RMSDiff, -d.RMSDB())
}
