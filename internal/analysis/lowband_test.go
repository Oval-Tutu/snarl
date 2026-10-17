package analysis

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/scenario"
)

// pulseTrain returns four Hann pulses per engine cycle at rpm, each pulse
// scaled by gain(cycle).
func pulseTrain(rate int, rpm, seconds float64, gain func(cycle int) float64) []float64 {
	x := make([]float64, int(seconds*float64(rate)))
	cycle := float64(rate) * 120 / rpm
	width := int(0.004 * float64(rate))
	for k := 0; ; k++ {
		g := gain(k)
		for p := range 4 {
			start := int(float64(k)*cycle + float64(p)*cycle/4)
			if start+width >= len(x) {
				return x
			}
			for i, w := range Hann(width) {
				x[start+i] += g * w
			}
		}
	}
}

func TestCycleVariationRandomGain(t *testing.T) {
	const rate, rpm = 48000, 1200.0
	rpmAt := func(float64) float64 { return rpm }
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // Seeded test signal, not security.
	periodic := BandPass(pulseTrain(rate, rpm, 3, func(int) float64 { return 1 }), rate, 20, 300)
	random := BandPass(pulseTrain(rate, rpm, 3, func(int) float64 { return 0.5 + r.Float64() }), rate, 20, 300)
	cvP, corrP, n, ok := CycleVariation(periodic[rate/5:], rate, rpmAt)
	if !ok || n < 20 {
		t.Fatalf("periodic: ok %v with %d cycles", ok, n)
	}
	cvR, _, _, ok := CycleVariation(random[rate/5:], rate, rpmAt)
	if !ok {
		t.Fatal("random gain: no cycle metrics")
	}
	if cvP > 0.01 || cvR < 0.1 {
		t.Errorf("energy CV: periodic %.4f, random gain %.4f", cvP, cvR)
	}
	if corrP < 0.99 {
		t.Errorf("periodic correlation %.4f, want near 1", corrP)
	}

	noisy := pulseTrain(rate, rpm, 3, func(int) float64 { return 1 })
	for i := range noisy {
		noisy[i] += 2 * r.NormFloat64()
	}
	_, corrN, _, _ := CycleVariation(BandPass(noisy, rate, 20, 300)[rate/5:], rate, rpmAt)
	if corrN > corrP-0.1 {
		t.Errorf("correlation: periodic %.4f, with noise %.4f", corrP, corrN)
	}
}

func TestHalfOrderDB(t *testing.T) {
	const rate, rpm = 48000, 3000.0
	rpmAt := func(float64) float64 { return rpm }
	whole := make([]float64, 2*rate)
	for _, o := range []float64{1, 2, 3, 4} {
		for i, v := range sine(o*rpm/60, 0.2, rate, 2) {
			whole[i] += v
		}
	}
	half := append([]float64(nil), whole...)
	for i, v := range sine(1.5*rpm/60, 0.2, rate, 2) {
		half[i] += v
	}
	orders := Orders(20)
	idx := BandOrders(orders, rpm, 20, 300)
	measure := func(x []float64) float64 {
		levels, ok := OrderSpectrum(x, rate, rpmAt, orders)
		if !ok {
			t.Fatal("no order spectrum")
		}
		h, ok := HalfOrderDB(orders, levels, idx)
		if !ok {
			t.Fatal("no half-order measure")
		}
		return h
	}
	hw, hh := measure(whole), measure(half)
	if hh-hw < 40 || math.Abs(hh-(-6.02)) > 0.5 {
		t.Errorf("half orders: whole only %.2f dB, with order 1.5 %.2f dB (want about -6.02)", hw, hh)
	}
}

func TestLowBandExcludesOutOfBandTone(t *testing.T) {
	const rate = 48000
	x := make([]float64, 3*rate)
	for _, f := range []float64{50, 100, 150} {
		for i, v := range sine(f, 0.2, rate, 3) {
			x[i] += v
		}
	}
	y := append([]float64(nil), x...)
	for i, v := range sine(1000, 0.2, rate, 3) {
		y[i] += v
	}
	sc, err := scenario.Parse(strings.NewReader("hold 3000 1 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Orders: Orders(20), Window: 0.5, Settle: 0.25, FFTSizes: DefaultFFTSizes, BandLo: 20, BandHi: 300}
	ms := Compare(x, y, rate, Windows(sc, 3, opts), opts)
	m := ms[0]
	if m.Order.Distance < 1 || m.ThirdOctaveLSD < 1 {
		t.Fatalf("full-band distances too small: order %.3f, third-octave %.3f", m.Order.Distance, m.ThirdOctaveLSD)
	}
	l := m.LowBand
	if l == nil || l.Order == nil {
		t.Fatal("no low-band metrics")
	}
	if l.Order.Orders[len(l.Order.Orders)-1] != 6 {
		t.Errorf("highest low-band order %v, want 6 (300 Hz at 3000 rpm)", l.Order.Orders[len(l.Order.Orders)-1])
	}
	if l.Order.Distance > 0.1 || l.ThirdOctaveLSD > 0.1 {
		t.Errorf("low-band distances include the 1 kHz tone: order %.3f, third-octave %.3f", l.Order.Distance, l.ThirdOctaveLSD)
	}
	if math.Abs(l.BalanceDB.A) > 0.1 || math.Abs(l.BalanceDB.B-(-1.25)) > 0.1 {
		t.Errorf("balance A %.2f dB (want 0), B %.2f dB (want -1.25)", l.BalanceDB.A, l.BalanceDB.B)
	}
}
