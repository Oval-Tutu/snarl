package model

import (
	"math"
	"testing"
)

// levelPeak runs a 200 Hz train of 1 ms raised-cosine pulses of the
// amplitude a through a new leveller for 1 s and returns the output peak
// over the last 0.5 s.
func levelPeak(a float64) float64 {
	l := newLeveller(rate)
	period, width := rate/200, rate/1000
	var peak float64
	for i := range rate {
		var x float64
		if j := i % period; j < width {
			x = a * (0.5 - 0.5*math.Cos(2*math.Pi*float64(j)/float64(width)))
		}
		y := math.Abs(float64(l.process(float32(x))))
		if i >= rate/2 {
			peak = max(peak, y)
		}
	}
	return peak
}

// TestLeveller checks the port of the engine-sim leveller at 48 kHz. Above
// the knee, an input 26 dB and 48 dB over the knee gives output peaks
// within 1 dB of each other, and from 0 to 2 dB over the target, because
// the gain smoothing lets each pulse overshoot: engine-sim gives 2.2 to 4.3
// dB of output change over 26 to 48 dB of input change on its engines,
// whose crest factor also changes. Below the knee, the gain is the largest
// gain. The peak decays by 8.7 dB in 22.7 ms, as 0.999 per sample does at
// 44.1 kHz.
func TestLeveller(t *testing.T) {
	knee := float64(levTarget / levMaxGain)
	db := func(x float64) float64 { return 20 * math.Log10(x) }
	lo, hi := levelPeak(knee*math.Pow(10, 26.0/20)), levelPeak(knee*math.Pow(10, 48.0/20))
	t.Logf("output peak %.2f dB and %.2f dB over the target at 26 dB and 48 dB over the knee", db(lo/levTarget), db(hi/levTarget))
	if math.Abs(db(hi/lo)) > 1 || db(lo/levTarget) < 0 || db(lo/levTarget) > 2 {
		t.Errorf("output peaks %.3f and %.3f, want within 1 dB of each other and 0 to 2 dB over %.3f", lo, hi, levTarget)
	}
	a := knee / 2
	if g := levelPeak(a) / a; math.Abs(db(g/levMaxGain)) > 0.05 {
		t.Errorf("gain %.3f below the knee, want %.1f", g, levMaxGain)
	}
	l := newLeveller(rate)
	l.process(1)
	n := int(math.Round(0.0227 * rate))
	for range n - 1 {
		l.process(0)
	}
	if d := db(float64(l.peak)); math.Abs(d+8.7) > 0.1 {
		t.Errorf("peak %.2f dB after %d samples, want -8.7 dB", d, n)
	}
}
