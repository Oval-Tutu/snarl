package analysis

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/scenario"
)

// TestSpectralStats checks the centroid, flatness and rolloff of a tone,
// which are its frequency and near 0, and of white noise, which are about
// a quarter and 0.85 of the sample rate and near 0.85, the ratio of the geometric to the
// arithmetic mean of Rayleigh magnitudes.
func TestSpectralStats(t *testing.T) {
	const rate = 48000
	c, f, r := SpectralStats(sine(1000, 0.5, rate, 1), rate)
	if math.Abs(c-1000) > 30 || math.Abs(r-1000) > 30 || f > 0.01 {
		t.Errorf("tone: centroid %.1f, rolloff %.1f, flatness %.4f, want about 1000, 1000 and below 0.01", c, r, f)
	}
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // Seeded test signal, not security.
	x := make([]float64, rate)
	for i := range x {
		x[i] = 0.1 * rng.NormFloat64()
	}
	c, f, r = SpectralStats(x, rate)
	if math.Abs(c-12000) > 300 || math.Abs(r-20400) > 300 || math.Abs(f-0.85) > 0.02 {
		t.Errorf("noise: centroid %.1f, rolloff %.1f, flatness %.4f, want about 12000, 20400 and 0.85", c, r, f)
	}
	if c, f, r := SpectralStats(make([]float64, 10), rate); c != 0 || f != 0 || r != 0 {
		t.Errorf("short input: got %v %v %v, want zeros", c, f, r)
	}
}

// TestDescribe checks the levels, the bands and the order spectrum of a
// 100 Hz tone of amplitude 0.5 in a hold at 3000 RPM, where 100 Hz is
// order 2.
func TestDescribe(t *testing.T) {
	const rate = 48000
	sc, err := scenario.Parse(strings.NewReader("hold 3000 1 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	x := sine(100, 0.5, rate, 2)
	ws := Windows(sc, 2, Options{Settle: 0.25, Window: 1})
	ds := Describe(x, rate, ws, Orders(4))
	if len(ds) != 1 {
		t.Fatalf("got %d descriptions, want 1", len(ds))
	}
	d := ds[0]
	near := func(name string, got, want, tol float64) {
		if math.Abs(got-want) > tol {
			t.Errorf("%s: got %.3f, want %.3f", name, got, want)
		}
	}
	near("RMS", d.RMSDBFS, -9.03, 0.05)
	near("peak", d.PeakDBFS, -6.02, 0.05)
	near("crest", d.CrestDB, 3.01, 0.05)
	// K-weighting attenuates 100 Hz by about 1.1 dB, and the loudness
	// offset is -0.691 dB.
	near("loudness", d.LoudnessLUFS, -9.03-1.1-0.691, 0.3)
	near("band 88-177", d.Bands[2], 0, 0.1)
	if d.Bands[0] > -40 || d.Bands[5] > -40 {
		t.Errorf("bands far from 100 Hz: %.1f and %.1f dB, want below -40", d.Bands[0], d.Bands[5])
	}
	if d.Orders == nil {
		t.Fatal("no order spectrum")
	}
	near("order 2", d.Orders[3], -6.02, 0.2)
	if d.Orders[1] > -40 {
		t.Errorf("order 1: %.1f dBFS, want below -40", d.Orders[1])
	}
}
