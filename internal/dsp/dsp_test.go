package dsp

import (
	"math"
	"testing"
)

const rate = 48000

// gain returns the steady-state gain of process at f hertz.
func gain(process func(float32) float32, f float64) float64 {
	var peak float64
	for i := range rate {
		y := process(float32(math.Cos(2 * math.Pi * f * float64(i) / rate)))
		if i > rate/2 {
			peak = math.Max(peak, math.Abs(float64(y)))
		}
	}
	return peak
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestSoftClip(t *testing.T) {
	for _, x := range []float32{-1e9, -1.5, -0.3, 0, 0.3, 1.5, 1e9, float32(math.NaN())} {
		y := SoftClip(x)
		if y != y || y < -1 || y > 1 {
			t.Errorf("SoftClip(%v) = %v", x, y)
		}
	}
	if SoftClip(1.5) != 1 || SoftClip(0.1) < 0.099 {
		t.Errorf("curve: %v %v", SoftClip(1.5), SoftClip(0.1))
	}
}

func TestFilters(t *testing.T) {
	var lp Biquad
	lp.SetLowPass(1000, 0.7071, rate)
	if g := gain(lp.Process, 50); !near(g, 1, 0.01) {
		t.Errorf("low pass at 50 Hz: %v", g)
	}
	lp.Reset()
	if g := gain(lp.Process, 8000); g > 0.05 {
		t.Errorf("low pass at 8 kHz: %v", g)
	}
	var pk Biquad
	pk.SetPeak(400, 2, -12, rate)
	if g := gain(pk.Process, 400); !near(g, 0.251, 0.01) {
		t.Errorf("peak at 400 Hz: %v, want -12 dB", g)
	}
	pk.Reset()
	if g := gain(pk.Process, 8000); !near(g, 1, 0.02) {
		t.Errorf("peak at 8 kHz: %v, want 1", g)
	}
	var out Biquad
	out.SetOutlet(0.03, 565, rate)
	if g := gain(out.Process, 30); g > 0.05 {
		t.Errorf("outlet at 30 Hz: %v", g)
	}
	out.Reset()
	if g := gain(out.Process, 15000); g < 0.8 {
		t.Errorf("outlet at 15 kHz: %v", g)
	}
	sh := NewHighShelf(1000, 0.5, rate)
	if g := gain(sh.Process, 20); !near(g, 1, 0.02) {
		t.Errorf("shelf at 20 Hz: %v", g)
	}
	sh = NewHighShelf(1000, 0.5, rate)
	if g := gain(sh.Process, 15000); !near(g, 0.5, 0.05) {
		t.Errorf("shelf at 15 kHz: %v", g)
	}
	sh = NewLowShelf(200, 2, rate)
	if g := gain(sh.Process, 10); !near(g, 2, 0.05) {
		t.Errorf("low shelf at 10 Hz: %v", g)
	}
	sh = NewLowShelf(200, 2, rate)
	if g := gain(sh.Process, 10000); !near(g, 1, 0.02) {
		t.Errorf("low shelf at 10 kHz: %v", g)
	}
	dc := NewDCBlock(10, rate)
	if g := gain(dc.Process, 1000); !near(g, 1, 0.01) {
		t.Errorf("DC block at 1 kHz: %v", g)
	}
}

func TestDelay(t *testing.T) {
	d := NewDelay(5)
	for i := range 20 {
		d.Write(float32(i))
	}
	if d.Read(0) != 19 || d.Read(5) != 14 {
		t.Errorf("read %v %v", d.Read(0), d.Read(5))
	}
}

func TestNoise(t *testing.T) {
	a, b := NewNoise(7), NewNoise(7)
	var sum float64
	for range 100000 {
		x := a.Bipolar()
		if x != b.Bipolar() || x < -1 || x >= 1 {
			t.Fatalf("bipolar %v", x)
		}
		if u := a.Unipolar(); u < 0 || u >= 1 {
			t.Fatalf("unipolar %v", u)
		}
		b.Unipolar()
		sum += float64(x)
	}
	if math.Abs(sum/100000) > 0.01 {
		t.Errorf("mean %v", sum/100000)
	}
	if !near(float64(Sine(0.25)), 1, 1e-4) || !near(float64(Sine(0.5)), 0, 1e-4) {
		t.Errorf("sine %v %v", Sine(0.25), Sine(0.5))
	}
}
