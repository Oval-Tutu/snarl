package esynth

import (
	"errors"
	"fmt"
	"math"
)

// FloatDifference is a Difference of float outputs with the place of the
// peak difference.
type FloatDifference struct {
	Difference
	// PeakAt is the index of the peak difference.
	PeakAt int
}

// String formats the difference for a report.
func (d FloatDifference) String() string {
	if d.Differing == 0 {
		return fmt.Sprintf("%s, peak difference %.3g at sample %d", d.Difference, d.PeakDiff, d.PeakAt)
	}
	return fmt.Sprintf("%s, peak at sample %d", d.Difference, d.PeakAt)
}

// CompareFloat returns the difference of port from ref, both in 16-bit
// steps and of the same length. A sample differs when the difference
// exceeds half a step, the rounding of the int16 output of engine-sim.
func CompareFloat(port, ref []float64) (FloatDifference, error) {
	if len(port) != len(ref) {
		return FloatDifference{}, fmt.Errorf("the port output has %d samples, the reference %d", len(port), len(ref))
	}
	fd := FloatDifference{Difference: Difference{Samples: len(ref), First: -1}}
	d := &fd.Difference
	var sumDiff, sumSignal float64
	for i, r := range ref {
		e := port[i] - r
		if math.Abs(e) > 0.5 {
			if d.Differing == 0 {
				d.First, d.FirstPort, d.FirstRef = i, int16(math.Round(port[i])), int16(math.Round(r))
			}
			d.Differing++
		}
		if math.Abs(e) > d.PeakDiff {
			d.PeakDiff, fd.PeakAt = math.Abs(e), i
		}
		d.PeakSignal = max(d.PeakSignal, math.Abs(r))
		sumDiff += e * e
		sumSignal += r * r
	}
	if d.Samples > 0 {
		d.RMSDiff = math.Sqrt(sumDiff / float64(d.Samples))
		d.RMSSignal = math.Sqrt(sumSignal / float64(d.Samples))
	}
	return fd, nil
}

// ResampleRuntime converts x from the synthesiser rate to rate with the
// resampler of a Runtime with o, aligned as RenderRuntime aligns its
// output. It lets a comparison of a Runtime output with the exact port
// leave out the resampler.
func ResampleRuntime(x []float64, from int, o RuntimeOptions) ([]float64, error) {
	if o.ResamplerTaps == 0 {
		o.ResamplerTaps = DefaultResamplerTaps
	}
	rs, err := newRuntimeResampler(from, o.OutputSampleRate, o.ResamplerTaps, 1024)
	if err != nil {
		return nil, err
	}
	want := int(int64(len(x)) * int64(o.OutputSampleRate) / int64(from))
	out := make([]float64, 0, want+1024)
	in := make([]float32, 1024)
	y := make([]float32, 2048)
	for pos := 0; len(out) < want; pos += len(in) {
		clear(in)
		for i := range in {
			if pos+i < len(x) {
				in[i] = float32(x[pos+i])
			}
		}
		rs.Push(in)
		for {
			n := rs.Pull(y)
			for _, v := range y[:n] {
				out = append(out, float64(v))
			}
			if n < len(y) {
				break
			}
		}
	}
	return out[:want], nil
}

// Steps returns x times 32768, a float output in 16-bit steps.
func Steps(x []float32) []float64 {
	out := make([]float64, len(x))
	for i, v := range x {
		out[i] = float64(v) * 32768
	}
	return out
}

// StepsInt16 returns x as float64 16-bit steps.
func StepsInt16(x []int16) []float64 {
	out := make([]float64, len(x))
	for i, v := range x {
		out[i] = float64(v)
	}
	return out
}

// errRuntimeInput reports a render block of a record for which the
// runtime holds too little input.
var errRuntimeInput = errors.New("render block needs more input than the runtime holds")

// RenderRuntime runs the Runtime on the dump input in the order of Render:
// per frame the steps, then the render blocks of the record, each after
// the rand() calls that engine-sim made outside the synthesiser. Output
// sample i at the synthesiser rate is the output of input sample i, as in
// Render, and the output covers the same time as the Render output. It
// returns the output at the output rate as float32 from -1 to 1.
func RenderRuntime(d *Dump, irs [][]int16, o RuntimeOptions) ([]float32, error) {
	rand := NewRandom(1)
	p := SimulatorParameters(d.InputChannelCount, d.SimulationFrequency)
	volumes := make([]float64, len(d.Channels))
	for i, c := range d.Channels {
		volumes[i] = c.Volume
	}
	r, err := NewRuntime(p, irs, volumes, o, rand)
	if err != nil {
		return nil, err
	}
	r.SetAudioParameters(d.Audio)

	outRate := float64(p.AudioSampleRate)
	if o.OutputSampleRate != 0 {
		outRate = float64(o.OutputSampleRate)
	}
	samples := 0
	for _, f := range d.Frames {
		for _, b := range f.Blocks {
			samples += int(b[0])
		}
	}
	want := int(float64(samples) * outRate / p.AudioSampleRate)
	out := make([]float32, 0, want+r.block)
	chunk := make([]float32, 2*r.block)
	emit := func() {
		r.finish()
		if r.rs == nil {
			out = append(out, r.out...)
			r.outPos = r.block
			return
		}
		for {
			n := r.rs.Pull(chunk)
			out = append(out, chunk[:n]...)
			if n < len(chunk) {
				return
			}
		}
	}

	data := make([]float64, d.InputChannelCount)
	step := 0
	for fi, f := range d.Frames {
		r.SetInputSampleRate(d.InputSampleRate)
		for range f.Steps {
			for c := range data {
				data[c] = d.Inputs[c][step]
			}
			r.WriteInput(data)
			step++
		}
		for bi, b := range f.Blocks {
			rand.Skip(b[1])
			n := int(b[0])
			if r.InputSize() < n {
				return nil, fmt.Errorf("frame %d block %d: %w: %d samples, %d held", fi, bi, errRuntimeInput, n, r.InputSize())
			}
			for n > 0 {
				k := min(n, r.block-r.fill)
				r.pre(k)
				n -= k
				if r.fill == r.block {
					emit()
				}
			}
		}
	}
	if r.Overflows() != 0 {
		return nil, fmt.Errorf("the runtime dropped %d input samples", r.Overflows())
	}
	// The last block: the convolution input after the end is zero.
	for len(out) < want {
		if r.fill == 0 {
			r.pre(0)
		}
		r.fill = r.block
		emit()
	}
	return out[:want], nil
}
