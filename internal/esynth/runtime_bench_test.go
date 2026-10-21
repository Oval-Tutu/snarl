package esynth

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// The benchmarks measure the cost of one audio block of 256 frames at
// 48 kHz, 5.333 ms of sound, which is 235.2 samples at the synthesiser rate.
const (
	benchFrames     = 256
	benchOutputRate = 48000
	benchCoreBlock  = benchFrames * 44100.0 / benchOutputRate
)

var benchBlockSeconds = float64(benchFrames) / benchOutputRate

// syntheticIR returns an impulse response of taps samples that the
// engine-sim trim keeps whole. The cost of the convolution depends on the
// length only.
func syntheticIR(taps int, seed uint32) []int16 {
	ir := make([]int16, taps)
	x := seed*7919 + 1
	for i := range ir {
		x = x*1664525 + 1013904223
		v := int16(101 + x>>17%20000) //nolint:gosec // The value is below 20101.
		if x&1 == 0 {
			v = -v
		}
		ir[i] = v
	}
	return ir
}

// benchRuntime returns a Runtime of the i4-vtec excerpt parameters with the
// given number of channels and synthetic impulse responses of the given
// length, the same for all channels unless separate, and the input of the
// excerpt.
func benchRuntime(tb testing.TB, channels, taps int, separate bool, o RuntimeOptions) (*Runtime, [][]float64) {
	tb.Helper()
	d, err := LoadDump(filepath.Join("..", "..", "testdata", "esynth", "i4-vtec-noise"))
	if err != nil {
		tb.Fatal(err)
	}
	p := SimulatorParameters(channels, d.SimulationFrequency)
	irs := make([][]int16, channels)
	volumes := make([]float64, channels)
	seed := uint32(0)
	for i := range irs {
		irs[i] = syntheticIR(taps, seed)
		if separate {
			seed++
		}
		volumes[i] = d.Channels[0].Volume
	}
	r, err := NewRuntime(p, irs, volumes, o, NewRandom(1))
	if err != nil {
		tb.Fatal(err)
	}
	r.SetAudioParameters(d.Audio)
	r.SetInputSampleRate(d.InputSampleRate)
	return r, d.Inputs[:channels]
}

// feed writes input steps until the runtime holds the input of frames
// output samples and a block, and returns the next step.
func feed(r *Runtime, input [][]float64, step, frames int) int {
	need := int(float64(frames)*r.audioSampleRate/float64(outputRate(r))) + 1 + r.block
	var data [8]float64
	for r.InputSize() < need {
		for c := range input {
			data[c] = input[c][step%len(input[c])]
		}
		r.WriteInput(data[:len(input)])
		step++
	}
	return step
}

func outputRate(r *Runtime) int {
	if r.rs == nil {
		return int(r.audioSampleRate)
	}
	return benchOutputRate
}

func reportBlock(b *testing.B, elapsed time.Duration, blocks float64) {
	b.Helper()
	ns := float64(elapsed.Nanoseconds()) / blocks
	b.ReportMetric(ns, "ns/block")
	b.ReportMetric(100*ns*1e-9/benchBlockSeconds, "%RT")
}

var benchTaps = []int{1024, 1893, 2048, 4096, 5979, 6907}

// BenchmarkRuntime measures Render with the input stage per block of 256
// frames at 48 kHz.
func BenchmarkRuntime(b *testing.B) {
	for _, block := range []int{64, 128, 256, 512} {
		for _, taps := range benchTaps {
			for _, ch := range []struct {
				name     string
				n        int
				separate bool
			}{{"1ch", 1, false}, {"2ch", 2, false}, {"2ch-separate", 2, true}} {
				b.Run(fmt.Sprintf("block=%d/taps=%d/%s", block, taps, ch.name), func(b *testing.B) {
					r, input := benchRuntime(b, ch.n, taps, ch.separate, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
					out := make([]float32, benchFrames)
					step := 0
					b.ResetTimer()
					for b.Loop() {
						step = feed(r, input, step, benchFrames)
						if r.Render(out) != benchFrames {
							b.Fatal("input ran out")
						}
					}
					reportBlock(b, b.Elapsed(), float64(b.N))
				})
			}
		}
	}
}

// BenchmarkRuntimeStages measures each stage of a two-channel runtime with
// a shared impulse response, per block of 256 frames at 48 kHz.
func BenchmarkRuntimeStages(b *testing.B) {
	const taps = 6907
	for _, block := range []int{128, 256} {
		b.Run(fmt.Sprintf("block=%d/input", block), func(b *testing.B) {
			r, input := benchRuntime(b, 2, taps, false, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
			step := 0
			samples := 0
			b.ResetTimer()
			for b.Loop() {
				before := r.InputSize()
				step = feed(r, input, step, benchFrames)
				samples += r.InputSize() - before
				r.read = r.written
			}
			reportBlock(b, b.Elapsed(), float64(samples)/benchCoreBlock)
		})
		b.Run(fmt.Sprintf("block=%d/pre", block), func(b *testing.B) {
			r, input := benchRuntime(b, 2, taps, false, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
			feed(r, input, 0, 2*block)
			blocks := 0
			for b.Loop() {
				// The same input block again.
				r.pre(block)
				r.fill = 0
				r.read -= block
				blocks++
			}
			reportBlock(b, b.Elapsed(), float64(blocks*block)/benchCoreBlock)
		})
		b.Run(fmt.Sprintf("block=%d/convolution", block), func(b *testing.B) {
			r, _ := benchRuntime(b, 2, taps, false, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
			g := &r.groups[0]
			for i := range g.in {
				g.in[i] = float32(i%7) - 3
			}
			blocks := 0
			for b.Loop() {
				g.conv.Process(g.in, g.out)
				blocks++
			}
			reportBlock(b, b.Elapsed(), float64(blocks*block)/benchCoreBlock)
		})
		b.Run(fmt.Sprintf("block=%d/post", block), func(b *testing.B) {
			r, _ := benchRuntime(b, 2, taps, false, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
			blocks := 0
			for b.Loop() {
				for i := range r.out {
					r.out[i] = float32(i%50)*400 - 10000
				}
				r.post()
				blocks++
			}
			reportBlock(b, b.Elapsed(), float64(blocks*block)/benchCoreBlock)
		})
		b.Run(fmt.Sprintf("block=%d/resampler", block), func(b *testing.B) {
			r, _ := benchRuntime(b, 2, taps, false, RuntimeOptions{OutputSampleRate: benchOutputRate, Block: block})
			x := make([]float32, block)
			for i := range x {
				x[i] = float32(i%9) * 0.1
			}
			y := make([]float32, 2*block)
			produced := 0
			for b.Loop() {
				r.rs.Push(x)
				for {
					n := r.rs.Pull(y)
					produced += n
					if n < len(y) {
						break
					}
				}
			}
			reportBlock(b, b.Elapsed(), float64(produced)/benchFrames)
		})
	}
}

// BenchmarkPreParts splits the stages before the convolution of a
// two-channel runtime, per block of 256 frames at 48 kHz.
func BenchmarkPreParts(b *testing.B) {
	const n = 128
	r, _ := benchRuntime(b, 2, 1024, false, RuntimeOptions{Block: n})
	rnd := r.rnd[:4*n]
	air := r.airIn[:2*n]
	js := r.jitterIn[:n]
	per := float64(n) / benchCoreBlock
	b.Run("rand", func(b *testing.B) {
		for b.Loop() {
			r.rand.fill(rnd, r.rndWork)
		}
		reportBlock(b, b.Elapsed(), float64(b.N)*per)
	})
	b.Run("air", func(b *testing.B) {
		for b.Loop() {
			for i := range air {
				air[i] = float32(rnd[2*i+1])*r.invRandMax2 - 1
			}
			r.air.block(air, air)
		}
		reportBlock(b, b.Elapsed(), float64(b.N)*per)
	})
	b.Run("jitter-draw", func(b *testing.B) {
		for b.Loop() {
			for c := range r.channels {
				for i := range js {
					js[i] = r.channels[c].jitter.gen.uniformFast(jitterTaps-1) * r.jitterScale
				}
			}
		}
		reportBlock(b, b.Elapsed(), float64(b.N)*per)
	})
	b.Run("jitter-filter", func(b *testing.B) {
		for b.Loop() {
			for c := range r.channels {
				r.channels[c].jitter.lp.block(js, js)
			}
		}
		reportBlock(b, b.Elapsed(), float64(b.N)*per)
	})
}

// BenchmarkExactPort measures the exact port with the direct convolution
// per block of 256 frames at 44.1 kHz rate equivalents, two channels.
func BenchmarkExactPort(b *testing.B) {
	for _, prec := range []Precision{Float64, Float32} {
		for _, taps := range []int{1893, 6907} {
			b.Run(fmt.Sprintf("precision=%d/taps=%d", prec, taps), func(b *testing.B) {
				d, err := LoadDump(filepath.Join("..", "..", "testdata", "esynth", "i4-vtec-noise"))
				if err != nil {
					b.Fatal(err)
				}
				p := SimulatorParameters(2, d.SimulationFrequency)
				p.Precision = prec
				s := New(p, NewRandom(1))
				s.SetAudioParameters(d.Audio)
				for i := range 2 {
					s.InitializeImpulseResponse(syntheticIR(taps, 0), d.Channels[i].Volume, i)
				}
				s.ReadAudioOutput(make([]int16, s.AudioBufferSize()))
				buf := make([]int16, 4096)
				data := make([]float64, 2)
				step, samples := 0, 0
				for b.Loop() {
					s.SetInputSampleRate(d.InputSampleRate)
					for s.InputSize() < benchFrames*44100/benchOutputRate {
						for c := range data {
							data[c] = d.Inputs[c][step%len(d.Inputs[c])]
						}
						s.WriteInput(data)
						step++
					}
					s.EndInputBlock()
					n := s.RenderAudio()
					s.ReadAudioOutput(buf[:n])
					s.EndInputBlock()
					samples += n
				}
				reportBlock(b, b.Elapsed(), float64(samples)/benchCoreBlock)
			})
		}
	}
}
