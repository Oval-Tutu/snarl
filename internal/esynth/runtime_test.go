package esynth

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// TestRuntimeEquivalence checks the Runtime at 44.1 kHz against the
// engine-sim output, which the Float32 port reproduces bit for bit.
func TestRuntimeEquivalence(t *testing.T) {
	for _, name := range excerpts {
		t.Run(name, func(t *testing.T) {
			d, irs, ref := loadExcerpt(t, name)
			rt, err := RenderRuntime(d, irs, RuntimeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			diff, err := CompareFloat(Steps(rt), StepsInt16(ref))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s", diff)
			if -diff.PeakDB() < 60 || -diff.RMSDB() < 70 {
				t.Errorf("%s, want the peak difference 60 dB and the RMS difference 70 dB below the signal", diff)
			}
		})
	}
}

// TestRuntimeEquivalence48k checks the Runtime at 48 kHz against the
// engine-sim output resampled by analysis.Resample, an independent
// windowed-sinc resampler.
func TestRuntimeEquivalence48k(t *testing.T) {
	for _, name := range excerpts {
		t.Run(name, func(t *testing.T) {
			d, irs, ref := loadExcerpt(t, name)
			rt, err := RenderRuntime(d, irs, RuntimeOptions{OutputSampleRate: 48000})
			if err != nil {
				t.Fatal(err)
			}
			want := analysis.Resample(StepsInt16(ref), 44100, 48000)
			// The ends hold the start-up of both resamplers.
			n := min(len(rt), len(want)) - 64
			diff, err := CompareFloat(Steps(rt)[64:n], want[64:n])
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s", diff)
			if -diff.PeakDB() < 60 || -diff.RMSDB() < 70 {
				t.Errorf("%s, want the peak difference 60 dB and the RMS difference 70 dB below the signal", diff)
			}
		})
	}
}

// TestRuntimeShortened checks that a shortened impulse response gives the
// requested taps.
func TestRuntimeShortened(t *testing.T) {
	ir := make([]int16, 3000)
	for i := range ir {
		ir[i] = 1000
	}
	h := runtimeImpulseResponse(ir, 1, 1024, 0)
	if len(h) != 1024 || h[1023] >= h[1024-128-1]*0.01 || h[1024-128-1] != h[0] {
		t.Errorf("%d taps, last %g, before the fade %g, first %g", len(h), h[len(h)-1], h[1024-128-1], h[0])
	}
	if h := runtimeImpulseResponse(ir, 1, 4096, 0); len(h) != 3000 {
		t.Errorf("%d taps, want the trimmed 3000", len(h))
	}
}

// TestReciprocal checks the Newton reciprocal of the leveller.
func TestReciprocal(t *testing.T) {
	for _, a := range []float32{float32Min, 1e-20, 0.3, 1, 1.5, 2, 3.999, 1234.5, 30000, 32767, 1e30} {
		got := reciprocal(a)
		want := 1 / a
		if e := (got - want) / want; e > 3e-7 || e < -3e-7 {
			t.Errorf("reciprocal(%g) = %g, want %g", a, got, want)
		}
	}
	if got := reciprocal(1e-40); got != posInf32 {
		t.Errorf("reciprocal of a subnormal = %g, want +Inf", got)
	}
}

// TestRandomFill checks the batch rand() draws against the single draws,
// across Skip and the wrap of the state.
func TestRandomFill(t *testing.T) {
	a, b := NewRandom(1), NewRandom(1)
	work := make([]uint32, 1000)
	dst := make([]int32, 900)
	for round, n := range []int{1, 2, 30, 31, 32, 500, 900, 7} {
		a.Skip(int64(round))
		b.Skip(int64(round))
		a.fill(dst[:n], work)
		for i := range n {
			if want := b.Rand(); dst[i] != want {
				t.Fatalf("round %d draw %d: %d, want %d", round, i, dst[i], want)
			}
		}
		if a.Calls() != b.Calls() || a.Rand() != b.Rand() {
			t.Fatalf("round %d: the generators part after the batch", round)
		}
	}
}

// TestUniformFast checks the jitter draw without division against the
// port.
func TestUniformFast(t *testing.T) {
	a, b := newMinstdRand0(), newMinstdRand0()
	for i := range 200000 {
		if x, y := a.uniformFast(9), b.uniformFloat32(0, 9); x != y {
			t.Fatalf("draw %d: %g, the port %g", i, x, y)
		}
	}
	// The port draws from [a, b) for any a. The runtime needs a = 0 only.
	if x, y := a.uniformFast(7)+2, b.uniformFloat32(2, 9); x != y {
		t.Errorf("draw from [2, 9): %g, the port %g", x, y)
	}
}

// TestRuntimeSide checks the side input: without the leveller, a main
// input x with a side input y renders what a main input x+y renders, as
// the stages are linear in the input and share the random draws, and the
// side input that stops and starts again starts from rest. With the
// leveller, the side input takes the gain of the main input alone, so it
// does not change the gain.
func TestRuntimeSide(t *testing.T) {
	sum, input := benchRuntime(t, 2, 1893, false, RuntimeOptions{OutputSampleRate: 48000, Unlevelled: true})
	split, _ := benchRuntime(t, 2, 1893, false, RuntimeOptions{OutputSampleRate: 48000, Unlevelled: true, Side: true})
	plain, _ := benchRuntime(t, 2, 1893, false, RuntimeOptions{OutputSampleRate: 48000})
	levelled, _ := benchRuntime(t, 2, 1893, false, RuntimeOptions{OutputSampleRate: 48000, Side: true})
	// y is a burst in the steps from 3000 to 3600 and from 9000 to 9100.
	y := func(step int) float64 {
		if step >= 3000 && step < 3600 || step >= 9000 && step < 9100 {
			return 3000 * math.Sin(float64(step)*0.3)
		}
		return 0
	}
	a, b, c, d := make([]float32, 256), make([]float32, 256), make([]float32, 256), make([]float32, 256)
	var data, main, side [2]float64
	step := 0
	var diff, peak, gain float64
	for range 120 {
		for sum.InputSize() < 256+300 {
			for ch := range 2 {
				x := input[ch][step%len(input[ch])]
				data[ch], main[ch], side[ch] = x+y(step), x, y(step)
			}
			sum.WriteInput(data[:])
			plain.WriteInput(main[:])
			// The side input runs from before each burst until its impulse
			// response has rung out.
			on := step >= 2900 && step < 5000 || step >= 8900 && step < 11000
			if on {
				split.WriteInputSide(main[:], side[:])
				levelled.WriteInputSide(main[:], side[:])
			} else {
				split.WriteInputSide(main[:], nil)
				levelled.WriteInputSide(main[:], nil)
			}
			step++
		}
		sum.Render(a)
		split.Render(b)
		plain.Render(c)
		levelled.Render(d)
		for i := range a {
			diff = max(diff, math.Abs(float64(a[i]-b[i])))
			peak = max(peak, math.Abs(float64(a[i])))
		}
		gain = max(gain, math.Abs(float64(plain.LevellerGain()-levelled.LevellerGain())))
	}
	if diff > 1e-4*peak || gain != 0 {
		t.Errorf("side input differs by %.3g of the peak %.3g, leveller gains differ by %g", diff/peak, peak, gain)
	}
}

// TestRuntimeNoAlloc checks that WriteInput and Render do not allocate.
func TestRuntimeNoAlloc(t *testing.T) {
	for _, rate := range []int{44100, 48000} {
		r, input := benchRuntime(t, 2, 6907, false, RuntimeOptions{OutputSampleRate: rate})
		out := make([]float32, 256)
		step := 0
		allocs := testing.AllocsPerRun(200, func() {
			step = feed(r, input, step, len(out))
			r.Render(out)
		})
		if allocs != 0 {
			t.Errorf("%d Hz: %g allocations per block", rate, allocs)
		}
	}
}
