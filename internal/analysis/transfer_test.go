package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestTransfer checks that an output of half the input, 30 samples late,
// with independent noise 20 dB below it, gives the lag, a gain of -6 dB in
// every band, and noise of -20 dB.
func TestTransfer(t *testing.T) {
	const rate, lag = 48000, 30
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // Seeded test signal, not security.
	n := 3 * rate
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range x {
		x[i] = rng.NormFloat64()
	}
	for i := range y {
		if i >= lag {
			y[i] = 0.5 * x[i-lag]
		}
		y[i] += 0.05 * rng.NormFloat64()
	}
	bands := ThirdOctaveBands(rate)
	l, ok := Lag(x, y, rate, 4096, 100, 8)
	if !ok || l != lag {
		t.Fatalf("lag %d (ok %v), want %d", l, ok, lag)
	}
	r, ok := Transfer(x, y, rate, 4096, l, bands)
	if !ok {
		t.Fatal("no transfer")
	}
	if math.Abs(r.NoiseDB+20) > 1 {
		t.Errorf("noise %.2f dB, want -20", r.NoiseDB)
	}
	for _, b := range r.Bands {
		if b.Lo < 100 {
			// Few bins: the estimate scatters more.
			continue
		}
		if math.Abs(b.GainDB+6.02) > 0.3 || math.Abs(b.NoiseDB+20) > 3 {
			t.Errorf("band %.0f-%.0f Hz: gain %.2f dB, noise %.2f dB, want -6.02 and -20", b.Lo, b.Hi, b.GainDB, b.NoiseDB)
		}
	}
	if _, ok := Transfer(x[:1000], y[:1000], rate, 4096, 0, bands); ok {
		t.Error("short input: got a transfer")
	}
}

// TestTransferNegativeLag checks an output that leads the input.
func TestTransferNegativeLag(t *testing.T) {
	const rate = 48000
	rng := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // Seeded test signal, not security.
	y := make([]float64, 2*rate)
	for i := range y {
		y[i] = rng.NormFloat64()
	}
	x := make([]float64, len(y))
	copy(x[12:], y)
	l, ok := Lag(x, y, rate, 2048, 100, 8)
	if !ok || l != -12 {
		t.Fatalf("lag %d (ok %v), want -12", l, ok)
	}
	r, ok := Transfer(x, y, rate, 2048, l, nil)
	if !ok || r.NoiseDB > -40 {
		t.Errorf("noise %.1f dB (ok %v), want below -40", r.NoiseDB, ok)
	}
}

// TestThirdOctaveBands checks the first band and the last band at 48 kHz.
func TestThirdOctaveBands(t *testing.T) {
	b := ThirdOctaveBands(48000)
	if len(b) != 30 || math.Abs(math.Sqrt(b[0][0]*b[0][1])-25.1) > 0.1 || b[len(b)-1][1] > 24000 {
		t.Errorf("got %d bands from %v to %v", len(b), b[0], b[len(b)-1])
	}
}
