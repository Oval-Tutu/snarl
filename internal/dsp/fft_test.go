package dsp

import (
	"fmt"
	"math"
	"math/cmplx"
	"testing"
)

func testSignal(n int) []float32 {
	x := make([]float32, n)
	s := NewNoise(7)
	for i := range x {
		x[i] = s.Bipolar()
	}
	return x
}

// TestRealFFT checks Forward against the direct transform and Inverse
// against Forward.
func TestRealFFT(t *testing.T) {
	for _, n := range []int{8, 16, 64, 512} {
		f, err := NewRealFFT(n)
		if err != nil {
			t.Fatal(err)
		}
		x := testSignal(n)
		spec := make([]complex64, n/2+1)
		f.Forward(x, spec)
		for k := range spec {
			var want complex128
			for j, v := range x {
				want += complex(float64(v), 0) * cmplx.Exp(complex(0, -2*math.Pi*float64(j*k)/float64(n)))
			}
			if e := cmplx.Abs(complex128(spec[k]) - want); e > 1e-5*float64(n) {
				t.Fatalf("n %d bin %d: %v, want %v", n, k, spec[k], want)
			}
		}
		y := make([]float32, n)
		f.Inverse(spec, y)
		for i := range y {
			if e := math.Abs(float64(y[i])/float64(n) - float64(x[i])); e > 1e-5 {
				t.Fatalf("n %d sample %d: %g, want %g", n, i, y[i]/float32(n), x[i])
			}
		}
	}
	if _, err := NewRealFFT(12); err == nil {
		t.Error("length 12 accepted")
	}
}

// TestConvolver checks the partitioned convolution against the direct
// convolution, for impulse responses shorter and longer than a block.
func TestConvolver(t *testing.T) {
	for _, c := range []struct{ taps, block int }{{5, 16}, {16, 16}, {100, 16}, {1893, 128}, {700, 64}} {
		t.Run(fmt.Sprintf("%d taps, block %d", c.taps, c.block), func(t *testing.T) {
			ir := testSignal(c.taps)
			conv, err := NewConvolver(ir, c.block)
			if err != nil {
				t.Fatal(err)
			}
			x := testSignal(c.taps + 5*c.block)
			y := make([]float32, len(x)/c.block*c.block)
			for i := 0; i+c.block <= len(x); i += c.block {
				conv.Process(x[i:i+c.block], y[i:i+c.block])
			}
			worst := 0.0
			for n := range y {
				var want float64
				for k, h := range ir {
					if n-k >= 0 {
						want += float64(h) * float64(x[n-k])
					}
				}
				worst = max(worst, math.Abs(float64(y[n])-want))
			}
			if worst > 1e-4 {
				t.Errorf("worst difference %g", worst)
			}
		})
	}
}

// BenchmarkConvolver measures one block. One partition is the cost of the
// two FFTs, and the rest is the multiply-accumulate per partition.
func BenchmarkConvolver(b *testing.B) {
	for _, block := range []int{64, 128, 256, 512} {
		for _, taps := range []int{block, 1024, 2048, 4096, 6907} {
			b.Run(fmt.Sprintf("block=%d/taps=%d", block, taps), func(b *testing.B) {
				conv, err := NewConvolver(testSignal(taps), block)
				if err != nil {
					b.Fatal(err)
				}
				x := testSignal(block)
				y := make([]float32, block)
				for b.Loop() {
					conv.Process(x, y)
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(block), "ns/sample")
			})
		}
	}
}

// TestResampler checks that a resampled sine is the sine at the output
// times, in the pass band of the 44.1 to 48 kHz design of esynth.
func TestResampler(t *testing.T) {
	for _, freq := range []float64{100, 1000, 5000, 10000, 16000} {
		rs, err := NewResampler(ResamplerConfig{From: 44100, To: 48000, Taps: 32, Cutoff: 0.475 * 44100, Beta: 8, MaxPush: 128})
		if err != nil {
			t.Fatal(err)
		}
		var y []float32
		buf := make([]float32, 256)
		x := make([]float32, 128)
		for b := range 100 {
			for i := range x {
				x[i] = float32(math.Sin(2 * math.Pi * freq * float64(b*128+i) / 44100))
			}
			rs.Push(x)
			for {
				n := rs.Pull(buf)
				y = append(y, buf[:n]...)
				if n < len(buf) {
					break
				}
			}
		}
		var sumE, sumS float64
		for j := 200; j < len(y); j++ {
			want := math.Sin(2 * math.Pi * freq * float64(j) / 48000)
			sumE += (float64(y[j]) - want) * (float64(y[j]) - want)
			sumS += want * want
		}
		db := 10 * math.Log10(sumE/sumS)
		limit := -60.0
		if freq > 12000 {
			limit = -25
		}
		if db > limit {
			t.Errorf("%g Hz: error %.1f dB, want below %.0f dB", freq, db, limit)
		}
		t.Logf("%g Hz: error %.1f dB", freq, db)
	}
}
