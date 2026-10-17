package analysis

import (
	"math"
	"math/cmplx"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/scenario"
)

func sine(freq, amp float64, rate int, seconds float64) []float64 {
	x := make([]float64, int(seconds*float64(rate)))
	for i := range x {
		x[i] = amp * math.Sin(2*math.Pi*freq*float64(i)/float64(rate))
	}
	return x
}

func TestFFTKnownSignal(t *testing.T) {
	const n = 64
	x := make([]complex128, n)
	for i := range x {
		ph := 2 * math.Pi * float64(i) / n
		x[i] = complex(1+2*math.Cos(5*ph)+0.5*math.Sin(9*ph), 0)
	}
	FFT(x)
	want := map[int]complex128{
		0:     n,
		5:     n,
		n - 5: n,
		9:     complex(0, -0.25*n),
		n - 9: complex(0, 0.25*n),
	}
	for k, v := range x {
		if d := cmplx.Abs(v - want[k]); d > 1e-9 {
			t.Errorf("bin %d: got %v, want %v", k, v, want[k])
		}
	}
}

func TestIntegratedLoudnessTone(t *testing.T) {
	// BS.1770: a 0 dBFS 1 kHz sine in one channel reads -3.01 LUFS, so a
	// -20 dBFS sine reads -23.01 LUFS.
	for _, rate := range []int{44100, 48000} {
		x := sine(1000, 0.1, rate, 5)
		got := IntegratedLoudness([][]float64{x}, rate)
		if math.Abs(got-(-23.01)) > 0.05 {
			t.Errorf("%d Hz: got %.3f LUFS, want -23.01", rate, got)
		}
	}
	if got := IntegratedLoudness([][]float64{make([]float64, 48000)}, 48000); !math.IsInf(got, -1) {
		t.Errorf("silence: got %v, want -Inf", got)
	}
}

func TestOrderSpectrumPulseTrain(t *testing.T) {
	// Three Hann pulses per revolution at 3000 rpm: 150 Hz, order 3.
	const rate, rpm = 48000, 3000.0
	x := make([]float64, 2*rate)
	period := rate * 60 / rpm / 3
	width := int(0.002 * rate)
	for start := 0.0; int(start)+width < len(x); start += period {
		for i, w := range Hann(width) {
			x[int(start)+i] += w
		}
	}
	orders := Orders(20)
	levels, ok := OrderSpectrum(x, rate, func(float64) float64 { return rpm }, orders)
	if !ok {
		t.Fatal("no order spectrum")
	}
	if d := DominantOrder(orders, levels); d != 3 {
		t.Fatalf("dominant order %v, want 3", d)
	}
	at := func(o float64) float64 { return levels[int(o*2)-1] }
	for _, o := range []float64{1, 1.5, 2, 4, 4.5} {
		if at(3)-at(o) < 30 {
			t.Errorf("order %v is %.1f dB, only %.1f dB below order 3", o, at(o), at(3)-at(o))
		}
	}
}

func TestOrderSpectrumSweepTone(t *testing.T) {
	// A tone that follows order 4 of a sweep from 2000 to 4000 rpm.
	const rate = 48000
	rpmAt := func(t float64) float64 { return 2000 + 2000*t }
	x := make([]float64, rate)
	var phase float64
	for i := range x {
		x[i] = 0.5 * math.Sin(2*math.Pi*phase)
		phase += 4 * rpmAt(float64(i)/rate) / 60 / rate
	}
	orders := Orders(20)
	levels, ok := OrderSpectrum(x, rate, rpmAt, orders)
	if !ok {
		t.Fatal("no order spectrum")
	}
	if d := DominantOrder(orders, levels); d != 4 {
		t.Fatalf("dominant order %v, want 4", d)
	}
	if got, want := levels[7], 20*math.Log10(0.5); math.Abs(got-want) > 1 {
		t.Errorf("order 4 level %.2f dB, want %.2f", got, want)
	}
}

func TestIdenticalInputsGiveZeroDistance(t *testing.T) {
	const rate = 48000
	x := sine(150, 0.3, rate, 3)
	for i := range x {
		x[i] += 0.1 * math.Sin(float64(i)*0.37) * math.Sin(float64(i)*0.0011)
	}
	sc, err := scenario.Parse(strings.NewReader("hold 3000 0.5 1\nsweep 2000 4000 0.5 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Orders: Orders(20), Window: 0.5, Settle: 0.25, FFTSizes: DefaultFFTSizes}
	windows := Windows(sc, 3, opts)
	if len(windows) != 5 {
		t.Fatalf("got %d windows, want 1 hold and 4 sweep windows", len(windows))
	}
	ms := Compare(x, append([]float64(nil), x...), rate, windows, opts)
	for _, m := range ms {
		if m.Order == nil {
			t.Fatalf("%s: no order metrics", m.Label)
		}
		for _, name := range MetricNames {
			if v, _ := MetricValue(&m, name); v != 0 {
				t.Errorf("%s: %s = %v, want 0", m.Label, name, v)
			}
		}
	}

	y := sine(200, 0.3, rate, 3)
	ms = Compare(x, y, rate, windows, opts)
	for _, m := range ms {
		if m.Order.Distance < 1 || m.ThirdOctaveLSD < 1 || m.MRSTFT.Total < 0.1 {
			t.Errorf("%s: distances too small for different inputs: %+v", m.Label, m)
		}
	}
}

func TestResampleKeepsTone(t *testing.T) {
	x := sine(1000, 0.5, 44100, 1)
	y := Resample(x, 44100, 48000)
	if len(y) != 48000 {
		t.Fatalf("got %d samples, want 48000", len(y))
	}
	want := sine(1000, 0.5, 48000, 1)
	for i := 100; i < len(y)-100; i++ {
		if math.Abs(y[i]-want[i]) > 1e-3 {
			t.Fatalf("sample %d: got %v, want %v", i, y[i], want[i])
		}
	}
}
