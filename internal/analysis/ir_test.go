package analysis

import (
	"math"
	"testing"
)

// resonator returns n samples of the impulse response of the two-pole
// filter 1/(1 - 2r cos(w) z^-1 + r^2 z^-2).
func resonator(r, w float64, n int) []float64 {
	h := make([]float64, n)
	a1, a2 := -2*r*math.Cos(w), r*r
	for i := range h {
		v := 0.0
		if i == 0 {
			v = 1
		}
		if i >= 1 {
			v -= a1 * h[i-1]
		}
		if i >= 2 {
			v -= a2 * h[i-2]
		}
		h[i] = v
	}
	return h
}

// TestAllPole checks that linear prediction of order 2 recovers the
// coefficients of a two-pole resonator, and that its response matches the
// response of the impulse response in every band.
func TestAllPole(t *testing.T) {
	const r, w = 0.99, 0.05
	h := resonator(r, w, 4000)
	a, g, ok := AllPole(h, 2)
	if !ok {
		t.Fatal("no fit")
	}
	if math.Abs(a[1]+2*r*math.Cos(w)) > 1e-3 || math.Abs(a[2]-r*r) > 1e-3 || math.Abs(g-1) > 0.01 {
		t.Errorf("got a %v and g %v, want [1 %v %v] and 1", a, g, -2*r*math.Cos(w), r*r)
	}
	bands := ThirdOctaveBands(44100)
	fir := IRResponse(h, 44100, bands)
	iir := AllPoleResponse(a, g, 44100, bands)
	for i := range bands {
		if math.Abs(fir[i]-iir[i]) > 0.2 {
			t.Errorf("band %v: FIR %.2f dB, all-pole %.2f dB", bands[i], fir[i], iir[i])
		}
	}
	if _, _, ok := AllPole(make([]float64, 10), 2); ok {
		t.Error("silence: got a fit")
	}
}

// TestIRTrimAndResponse checks the engine-sim trim of the tail and the cap
// on the length, and that a unit impulse has a flat 0 dB response.
func TestIRTrimAndResponse(t *testing.T) {
	ir := []float64{1, 0.5, 0.01, 0.001, 0.0001, 0}
	if got := IRTrim(ir, 100); len(got) != 3 {
		t.Errorf("trim: %d samples, want 3", len(got))
	}
	if got := IRTrim(ir, 2); len(got) != 2 {
		t.Errorf("cap: %d samples, want 2", len(got))
	}
	for i, v := range IRResponse([]float64{1}, 44100, ThirdOctaveBands(44100)) {
		if math.Abs(v) > 1e-9 {
			t.Errorf("unit impulse band %d: %.3f dB, want 0", i, v)
		}
	}
}

// TestSynthPathResponse checks the path of a unit impulse: at 1 kHz the
// first difference gives 20*log10(2*sin(pi*1000/44100)), -16.9 dB, and the
// low pass at 1.9 kHz almost nothing, and at 4 kHz the difference gives
// -5.0 dB and the low pass -25.8 dB.
func TestSynthPathResponse(t *testing.T) {
	bands := ThirdOctaveBands(44100)
	r := SynthPathResponse([]float64{1}, 44100, 1900, bands)
	// Bands 16 and 22 are centred on 1 kHz and 3981 Hz.
	if math.Abs(r[16]+16.9) > 0.3 || math.Abs(r[22]+30.8) > 1.5 {
		t.Errorf("1 kHz %.2f dB and 4 kHz %.2f dB, want -16.9 and -30.8", r[16], r[22])
	}
}

// TestEnergyDecay checks an exponential decay exp(-n/tau), whose energy
// falls 20 dB after tau*ln(100)/2 samples and 40 dB after twice that.
func TestEnergyDecay(t *testing.T) {
	const rate, tau = 44100, 441.0
	h := make([]float64, 20000)
	for i := range h {
		h[i] = math.Exp(-float64(i) / tau)
	}
	t20, t40 := EnergyDecay(h, rate)
	w20 := tau * math.Log(100) / 2 / rate
	if math.Abs(t20-w20) > 1e-4 || math.Abs(t40-2*w20) > 1e-4 {
		t.Errorf("got %.5f s and %.5f s, want %.5f s and %.5f s", t20, t40, w20, 2*w20)
	}
}
