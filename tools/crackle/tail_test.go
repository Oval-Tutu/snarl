package main

import (
	"math"
	"testing"
)

// decayTone returns a 500 Hz sine with the amplitude a(t) from 0 to 0.5 s,
// then 0.3 s of silence.
func decayTone(rate int, a func(t float64) float64) []float64 {
	x := make([]float64, int(0.8*float64(rate)))
	for i := range int(0.5 * float64(rate)) {
		t := float64(i) / float64(rate)
		x[i] = a(t) * math.Sin(2*math.Pi*500*t)
	}
	return x
}

func TestMeasureDecayExponential(t *testing.T) {
	const rate, tau = 44100, 0.02
	x := decayTone(rate, func(t float64) float64 { return math.Exp(-t / tau) })
	d := measureDecay(decayEnvs(x, rate)[0], rate, 0, len(x), len(x))
	// An amplitude of exp(-t/tau) falls 20/ln(10)/tau dB per second.
	slope := -20 / math.Ln10 / tau / 1000
	if math.Abs(d.Slope-slope) > 0.03*math.Abs(slope) {
		t.Errorf("slope %.3f dB/ms, want %.3f", d.Slope, slope)
	}
	// The 5 ms window adds about 2.5 ms at the start of the fall.
	for k, drop := range decayDrops {
		want := drop / -slope
		if got := d.DropMS[k]; math.Abs(got-want) > 3 {
			t.Errorf("time to -%g dB %.1f ms, want %.1f", drop, got, want)
		}
	}
	if d.TwoStage {
		t.Errorf("one exponential is two-stage: fast %.3f, slow %.3f dB/ms", d.Fast, d.Slow)
	}
	// The 20 to 100 ms window against the peak: the mean of exp(-2t/tau).
	want := powDB(tau / 2 * (math.Exp(-2*0.02/tau) - math.Exp(-2*0.1/tau)) / 0.08)
	if math.Abs(d.WinDB[0]-want) > 1.5 {
		t.Errorf("20 to 100 ms %.1f dB, want %.1f", d.WinDB[0], want)
	}
}

func TestMeasureDecayTwoStage(t *testing.T) {
	const rate, fast, slow = 44100, 0.002, 0.05
	x := decayTone(rate, func(t float64) float64 { return math.Exp(-t/fast) + 0.05*math.Exp(-t/slow) })
	d := measureDecay(decayEnvs(x, rate)[0], rate, 0, len(x), len(x))
	if !d.TwoStage {
		t.Fatalf("a fast and a slow exponential are one stage: one line %.3f, fast %.3f, slow %.3f dB/ms", d.Slope, d.Fast, d.Slow)
	}
	want := -20 / math.Ln10 / slow / 1000
	if math.Abs(d.Slow-want) > 0.1*math.Abs(want) {
		t.Errorf("slow slope %.3f dB/ms, want %.3f", d.Slow, want)
	}
	// The slow stage starts 26 dB down, where the two terms cross.
	if d.BreakDB > -15 || d.BreakDB < -35 {
		t.Errorf("break at %.1f dB, want near -26", d.BreakDB)
	}
}
