package esynth

import (
	"fmt"
	"math"
	"slices"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// RuntimeOptions configures a Runtime.
type RuntimeOptions struct {
	// OutputSampleRate is the rate of Render in hertz. At the synthesiser
	// rate, 44100 Hz for engine-sim, Render returns the synthesiser output.
	// At another rate a polyphase resampler converts it. The default is
	// the synthesiser rate.
	OutputSampleRate int
	// Block is the convolution block in samples at the synthesiser rate, a
	// power of two. The default is DefaultRuntimeBlock.
	Block int
	// MaxTaps shortens each impulse response after the engine-sim trim to
	// at most this many taps, with a raised-cosine fade over the last
	// FadeTaps taps. 0 keeps the trimmed length.
	MaxTaps int
	// FadeTaps is the fade of a shortened impulse response. The default is
	// MaxTaps/8.
	FadeTaps int
	// ResamplerTaps is the number of resampler coefficients per phase. The
	// default is DefaultResamplerTaps.
	ResamplerTaps int
	// InputBuffer is the capacity of the input buffer in samples at the
	// synthesiser rate, a power of two. The default is 8192.
	InputBuffer int
	// Unlevelled leaves out the leveller, the volume and the clamp: Render
	// returns the output filter output, in the units of PreLevelPower.
	Unlevelled bool
	// Mix, when set, adds to each block at the synthesiser rate after the
	// leveller, the volume and the clamp, and before the resampler. It
	// must not allocate.
	Mix func(block []float32)
}

const (
	// DefaultRuntimeBlock is the default convolution block of a Runtime.
	DefaultRuntimeBlock = 256
	// DefaultResamplerTaps is the default resampler length per phase.
	DefaultResamplerTaps = 32
	// resamplerCutoffRatio is the -6 dB frequency of the resampler as a
	// fraction of the lower rate.
	resamplerCutoffRatio = 0.475
	// resamplerBeta is the Kaiser window parameter of the resampler, for
	// about 80 dB of stop band attenuation.
	resamplerBeta = 8.0
	// jitterTaps is the history of the jitter filter, 10 samples in
	// Synthesizer::initialize.
	jitterTaps = 10
)

// Runtime runs the synthesiser of the port in float32 for real-time use. It
// takes the same input as Synthesizer and gives the same output within the
// tolerance in docs/research/esynth-runtime.md. It differs from the exact
// port in the arithmetic only:
//
//   - The convolution is a uniformly partitioned FFT convolution
//     (dsp.Convolver) instead of the direct form. Channels with the same
//     impulse response share one convolution, which is the same sum.
//   - The 1900 Hz input filter runs as two second-order sections instead
//     of a fourth-order direct form. It stays in double, as in engine-sim:
//     in float32 the difference from engine-sim grows from 4 to 11 16-bit
//     steps on the v12-merlin sweep.
//   - The coefficients of the filters are computed once, and the
//     divisions per sample of the port are multiplications by a stored
//     reciprocal. The leveller keeps the reciprocal of its peak.
//   - The output is float32 from -1 to 1 instead of int16, with the clamp
//     of engine-sim and without its rounding.
//
// The random numbers, the jitter and the leveller follow the port. Render
// and WriteInput do not allocate or lock, and they do not divide or call
// math.Sin, math.Exp or math.Pow per sample. A Runtime is not safe for
// concurrent use.
type Runtime struct {
	block    int
	channels []rtChannel
	// mask indexes the input ring of each channel.
	mask            int
	written, read   int
	overflow        int
	inputRing       int
	inputSampleRate float64
	audioSampleRate float64
	step, invStep   float64
	writeOffset     float64
	lastOffset      float64
	writeIndex      int

	rand *Random
	air  bw4
	// airCutoff is the cutoff of air that the coefficients were made for.
	airCutoff float64

	groups []rtGroup
	dry    []float32
	// Work buffers of pre.
	rnd      []int32
	rndWork  []uint32
	airIn    []float32
	jitterIn []float32
	fill     int
	// out holds the last block, of which outPos samples were read.
	out    []float32
	outPos int
	rs     *dsp.Resampler
	aa     bw4
	lev    rtLeveller
	// unlevelled skips the stages after the output filter, and prePower is
	// the mean square of the last block before the leveller.
	unlevelled bool
	prePower   float64
	invBlock   float32
	// mix adds to each block before the resampler (RuntimeOptions.Mix).
	mix func([]float32)

	audio                         AudioParameters
	airNoise, airDry, dFFMix, wet float32
	conv, dryAmount, volume       float32
	jitterScale                   float32
	invRandMax2                   float32
	invDt                         float32
	dcAlpha, dcBeta               float32
}

type rtGroup struct {
	conv *dsp.Convolver
	in   []float32
	out  []float32
}

type rtChannel struct {
	ring      []float32
	input     [2]sos
	lastInput float64
	jitter    rtJitter
	dc        float32
	previous  float32
	group     int
}

// NewRuntime returns a Runtime with the parameters of New and the
// impulse response and volume of each channel, as InitializeImpulseResponse
// takes them. rand is the rand() generator.
func NewRuntime(p Parameters, irs [][]int16, volumes []float64, o RuntimeOptions, rand *Random) (*Runtime, error) {
	if p.InputChannelCount < 1 || len(irs) != p.InputChannelCount || len(volumes) != p.InputChannelCount {
		return nil, fmt.Errorf("esynth: %d channels with %d impulse responses and %d volumes", p.InputChannelCount, len(irs), len(volumes))
	}
	if o.Block == 0 {
		o.Block = DefaultRuntimeBlock
	}
	if o.InputBuffer == 0 {
		o.InputBuffer = 8192
	}
	if o.ResamplerTaps == 0 {
		o.ResamplerTaps = DefaultResamplerTaps
	}
	if o.OutputSampleRate == 0 {
		o.OutputSampleRate = int(p.AudioSampleRate)
	}
	if o.InputBuffer&(o.InputBuffer-1) != 0 || o.InputBuffer < 2*o.Block {
		return nil, fmt.Errorf("esynth: input buffer %d is not a power of two of at least two blocks", o.InputBuffer)
	}
	if o.MaxTaps < 0 || o.FadeTaps < 0 || o.FadeTaps > o.MaxTaps {
		return nil, fmt.Errorf("esynth: %d taps with a fade of %d", o.MaxTaps, o.FadeTaps)
	}
	rate := p.AudioSampleRate
	r := &Runtime{
		block:           o.Block,
		channels:        make([]rtChannel, p.InputChannelCount),
		mask:            o.InputBuffer - 1,
		inputRing:       p.InputBufferSize,
		audioSampleRate: rate,
		rand:            rand,
		dry:             make([]float32, o.Block),
		rnd:             make([]int32, 2*p.InputChannelCount*o.Block),
		rndWork:         make([]uint32, 2*p.InputChannelCount*o.Block+31),
		airIn:           make([]float32, p.InputChannelCount*o.Block),
		jitterIn:        make([]float32, o.Block),
		out:             make([]float32, o.Block),
		outPos:          o.Block,
		aa:              newBW4(float64(float32(rate)*float32(0.45)), rate),
		// Synthesizer::initialize sets the gain limits from the initial
		// parameters only.
		lev: rtLeveller{
			peak: 30000, invPeak: 1.0 / 30000, attenuation: 1,
			minLevel: float32(p.InitialAudioParameters.LevelerMinGain),
			maxLevel: float32(p.InitialAudioParameters.LevelerMaxGain),
		},
		invRandMax2: float32(2.0 / RandMax),
		unlevelled:  o.Unlevelled,
		invBlock:    1 / float32(o.Block),
		mix:         o.Mix,
	}
	r.SetInputSampleRate(p.InputSampleRate)
	dt := float32(1) / float32(rate)
	r.invDt = 1 / dt
	dcRC := float32(1.0) / (float32(10.0*2.0) * float32(enginePiFloat))
	r.dcAlpha = dt / (dcRC + dt)
	r.dcBeta = 1 - r.dcAlpha

	sections := inputSections(float64(float32(1900.0)), rate)
	jitterLP := newBW4(p.InitialAudioParameters.InputSampleNoiseFrequencyCutoff, rate)
	for i := range r.channels {
		c := &r.channels[i]
		c.ring = make([]float32, o.InputBuffer)
		c.input = sections
		c.jitter = rtJitter{lp: jitterLP, gen: newMinstdRand0()}
	}
	r.air = newBW4(p.InitialAudioParameters.AirNoiseFrequencyCutoff, rate)
	r.airCutoff = p.InitialAudioParameters.AirNoiseFrequencyCutoff

	var taps [][]float32
	for i, ir := range irs {
		h := runtimeImpulseResponse(ir, volumes[i], o.MaxTaps, o.FadeTaps)
		if len(h) == 0 {
			return nil, fmt.Errorf("esynth: channel %d: the impulse response is empty after the trim", i)
		}
		g := slices.IndexFunc(taps, func(t []float32) bool { return slices.Equal(t, h) })
		if g < 0 {
			conv, err := dsp.NewConvolver(h, o.Block)
			if err != nil {
				return nil, err
			}
			g = len(taps)
			taps = append(taps, h)
			r.groups = append(r.groups, rtGroup{conv: conv, in: make([]float32, o.Block), out: make([]float32, o.Block)})
		}
		r.channels[i].group = g
	}

	if o.OutputSampleRate != int(rate) {
		rs, err := newRuntimeResampler(int(rate), o.OutputSampleRate, o.ResamplerTaps, o.Block)
		if err != nil {
			return nil, err
		}
		r.rs = rs
	}
	r.SetAudioParameters(p.InitialAudioParameters)
	return r, nil
}

// newRuntimeResampler designs the resampler of a Runtime.
func newRuntimeResampler(from, to, taps, maxPush int) (*dsp.Resampler, error) {
	return dsp.NewResampler(dsp.ResamplerConfig{
		From:    from,
		To:      to,
		Taps:    taps,
		Cutoff:  resamplerCutoffRatio * float64(min(from, to)),
		Beta:    resamplerBeta,
		MaxPush: maxPush,
	})
}

// runtimeImpulseResponse trims and scales an impulse response as
// InitializeImpulseResponse does in Float32 precision, then shortens it to
// maxTaps with a raised-cosine fade over the last fade taps.
func runtimeImpulseResponse(ir []int16, volume float64, maxTaps, fade int) []float32 {
	clipped := 0
	for i, v := range ir {
		if abs16(v) > 100 {
			clipped = i + 1
		}
	}
	n := min(10000, clipped)
	h := make([]float32, n)
	for i := range h {
		h[i] = float32(float32(volume*float64(ir[i])) / math.MaxInt16)
	}
	if maxTaps == 0 || maxTaps >= n {
		return h
	}
	if fade == 0 {
		fade = maxTaps / 8
	}
	h = h[:maxTaps]
	for i := maxTaps - fade; i < maxTaps; i++ {
		u := float64(i-(maxTaps-fade)+1) / float64(fade+1)
		h[i] *= float32(0.5 * (1 + math.Cos(math.Pi*u)))
	}
	return h
}

// SetAudioParameters ports Synthesizer::setAudioParameters. It computes
// the coefficients of the new parameters. A change of the air noise cutoff
// designs the filter again, which allocates.
func (r *Runtime) SetAudioParameters(a AudioParameters) {
	r.audio = a
	r.airNoise = float32(a.AirNoise)
	r.airDry = 1 - r.airNoise
	r.dFFMix = float32(a.DFFMix)
	r.wet = 1 - r.dFFMix
	r.conv = float32(a.Convolution)
	r.dryAmount = 1 - r.conv
	r.volume = float32(a.Volume)
	r.jitterScale = float32(a.InputSampleNoise)
	r.lev.target = float32(a.LevelerTarget)
	if cutoff := float64(float32(a.AirNoiseFrequencyCutoff)); cutoff != r.airCutoff {
		next := newBW4(cutoff, r.audioSampleRate)
		r.air.g, r.air.a1, r.air.a2, r.air.a3, r.air.a4 = next.g, next.a1, next.a2, next.a3, next.a4
		r.airCutoff = cutoff
	}
}

// AudioParameters returns the parameters of the last SetAudioParameters.
func (r *Runtime) AudioParameters() AudioParameters { return r.audio }

// SetInputSampleRate ports Synthesizer::setInputSampleRate.
func (r *Runtime) SetInputSampleRate(sampleRate float64) {
	rate := float64(float32(sampleRate))
	if rate == r.inputSampleRate {
		return
	}
	r.inputSampleRate = rate
	r.step = r.audioSampleRate / rate
	r.invStep = 1 / r.step
}

// Latency returns the delay of Render in samples at the synthesiser rate:
// the convolution block and the look-ahead of the resampler.
func (r *Runtime) Latency() int {
	if r.rs == nil {
		return r.block
	}
	return r.block + r.rs.Delay()
}

// InputSize returns the input samples at the synthesiser rate that the
// Runtime has not rendered.
func (r *Runtime) InputSize() int { return r.written - r.read }

// Overflows returns the input samples that WriteInput dropped because the
// input buffer was full.
func (r *Runtime) Overflows() int { return r.overflow }

// WriteInput ports Synthesizer::WriteInput: one input sample per channel
// at the input rate, interpolated linearly to the synthesiser rate and
// low-passed at 1900 Hz. The offsets stay in float64 as in the port, so
// that the samples fall at the same times. When the input buffer is full,
// WriteInput drops the new samples and counts them.
func (r *Runtime) WriteInput(data []float64) {
	r.writeOffset += r.step
	size := float64(r.inputRing)
	if r.writeOffset >= size {
		r.writeOffset -= size
	}
	distance := r.inputDistance(r.writeOffset, r.lastOffset)
	t0 := r.inputDistance(float64(r.writeIndex), r.lastOffset)
	n := 0
	for i := range r.channels {
		c := &r.channels[i]
		n = r.writeRing(c.ring, &c.input, &c.lastInput, data[i], t0, distance)
	}
	r.lastOffset = r.writeOffset
	r.writeIndex += n
	if r.writeIndex >= r.inputRing {
		r.writeIndex -= r.inputRing
	}
	kept := min(n, r.mask+1-(r.written-r.read))
	r.overflow += n - kept
	r.written += kept
}

// writeRing interpolates the input of one channel from its last sample to
// x over the input times t0 to distance, low-passes it at 1900 Hz, writes
// it to the ring from the write position, and returns the samples.
func (r *Runtime) writeRing(ring []float32, input *[2]sos, lastInput *float64, x, t0, distance float64) int {
	size := float64(r.inputRing)
	last := *lastInput
	w := r.written
	n := 0
	for t := t0; t <= distance; t += 1.0 {
		if t >= size {
			t -= size
		}
		f := t * r.invStep
		s := float32(last*(1-f) + x*f)
		s = float32(input[1].f(input[0].f(float64(s))))
		if w-r.read <= r.mask {
			ring[w&r.mask] = s
			w++
		}
		n++
	}
	*lastInput = x
	return n
}

func (r *Runtime) inputDistance(s1, s0 float64) float64 {
	if s1 < s0 {
		return float64(r.inputRing) - s0 + s1
	}
	return s1 - s0
}

// Render writes the output to out at the output rate and returns the
// samples that it wrote. It renders a convolution block when it needs one
// and the input holds enough samples. It writes fewer than len(out)
// samples only when the input runs out.
func (r *Runtime) Render(out []float32) int {
	n := 0
	for n < len(out) {
		if r.rs != nil {
			k := r.rs.Pull(out[n:])
			n += k
			if n == len(out) {
				break
			}
		} else if r.outPos < r.block {
			k := copy(out[n:], r.out[r.outPos:])
			r.outPos += k
			n += k
			continue
		}
		if r.InputSize() < r.block-r.fill {
			break
		}
		r.pre(r.block - r.fill)
		r.finish()
	}
	return n
}

// pre runs the stages before the convolution on the next n input samples,
// at most the rest of the block. engine-sim computes each sample for all
// channels in turn. pre computes the same values stage by stage over the
// samples: first the rand() draws in the engine-sim order, then the air
// noise filter over the noise of all channels in that order, then per
// channel the jitter, the DC removal, the derivative and the mix. Each
// filter then runs in one loop with its state in registers.
func (r *Runtime) pre(n int) {
	start := r.fill
	if start == 0 {
		for g := range r.groups {
			clear(r.groups[g].in)
		}
		clear(r.dry)
	}
	if n == 0 {
		return
	}
	nc := len(r.channels)

	// Per sample and channel, r_0 and the noise draw. engine-sim draws
	// r_0 and does not use it.
	rnd := r.rnd[:2*nc*n]
	r.rand.fill(rnd, r.rndWork)
	air := r.airIn[:nc*n]
	for i := range air {
		air[i] = float32(rnd[2*i+1])*r.invRandMax2 - 1
	}
	// engine-sim uses the air noise filter of channel 0 for every channel.
	r.air.block(air, air)

	for ci := range r.channels {
		c := &r.channels[ci]
		js := r.jitterIn[:n]
		for i := range js {
			js[i] = c.jitter.gen.uniformFast(jitterTaps-1) * r.jitterScale
		}
		c.jitter.lp.block(js, js)

		in := r.groups[c.group].in[start : start+n]
		dry := r.dry[start : start+n]
		dc, previous := c.dc, c.previous
		alpha, beta, invDt := r.dcAlpha, r.dcBeta, r.invDt
		airNoise, airDry, mix, wet := r.airNoise, r.airDry, r.dFFMix, r.wet
		// engine-sim mixes in the dry signal only for a convolution amount
		// below 1.
		dryAmount := r.dryAmount
		dryOn := dryAmount != 0
		read, mask, ring := r.read, r.mask, c.ring
		for i, s := range js {
			fIn := c.jitter.f(ring[(read+i)&mask], s)
			dc = alpha*fIn + beta*dc
			fAc := fIn - dc
			fP := (fIn - previous) * invDt
			previous = fIn

			rMixed := airNoise*air[i*nc+ci] + airDry
			vIn := fP*mix + fAc*rMixed*wet
			if vIn < float32Min && vIn > -float32Min {
				vIn = 0
			}
			in[i] += vIn
			if dryOn {
				dry[i] += dryAmount * vIn
			}
		}
		c.dc, c.previous = dc, previous
	}
	r.read += n
	r.fill += n
}

// finish runs the convolution and the stages after it on a full block,
// and hands the block to the resampler or to Render.
func (r *Runtime) finish() {
	out := r.out
	copy(out, r.dry)
	for g := range r.groups {
		gr := &r.groups[g]
		gr.conv.Process(gr.in, gr.out)
		for i, v := range gr.out[:len(out)] {
			out[i] += r.conv * v
		}
	}
	r.post()
	if r.mix != nil {
		r.mix(r.out)
	}
	r.fill = 0
	if r.rs != nil {
		r.rs.Push(r.out)
		r.outPos = r.block
		return
	}
	r.outPos = 0
}

// PreLevelPower returns the mean square of the last rendered block after
// the output filter and before the leveller, in the units of the
// Unlevelled output.
func (r *Runtime) PreLevelPower() float64 { return r.prePower }

// LevellerGain returns the gain of the leveller after the last rendered
// sample.
func (r *Runtime) LevellerGain() float32 { return r.lev.attenuation }

// post runs the output filter, the leveller, the volume and the clamp on
// the block in out.
func (r *Runtime) post() {
	out := r.out
	r.aa.block(out, out)
	var sq float32
	for _, s := range out {
		sq += s * s
	}
	r.prePower = float64(sq * r.invBlock)
	if r.unlevelled {
		return
	}
	volume := r.volume
	for i, s := range out {
		v := r.lev.f(s) * volume
		v = min(max(v, math.MinInt16), math.MaxInt16)
		out[i] = v * (1.0 / 32768)
	}
}

// bw4 is ButterworthLowPassFilter<float> with its coefficients computed
// once: g is f^4 / a0, which fast_f computes per sample.
type bw4 struct {
	g, a1, a2, a3, a4 float32
	x1, x2, x3, x4    float32
	y1, y2, y3, y4    float32
}

// newBW4 designs the filter as the port does in Float32 precision.
func newBW4(fc, rate float64) bw4 {
	b := newButterworthLowPass(true, Float32)
	b.setCutoffFrequency(fc, rate)
	return bw4{
		g:  float32(float32(b.f4) / float32(b.a[0])),
		a1: float32(b.a[1]),
		a2: float32(b.a[2]),
		a3: float32(b.a[3]),
		a4: float32(b.a[4]),
	}
}

// sos is a second-order section in transposed direct form II in double.
type sos struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             float64
}

func (s *sos) f(x float64) float64 {
	y := s.b0*x + s.z1
	s.z1 = s.b1*x - s.a1*y + s.z2
	s.z2 = s.b2*x - s.a2*y
	return y
}

// inputSections splits ButterworthLowPassFilter<double> into its two
// second-order sections: with f = tan(pi fc / rate) and the pole terms m
// and n of setCutoffFrequency, section k is f^2 (1 + z^-1)^2 over
// (1 + k f + f^2) + (2 f^2 - 2) z^-1 + (1 - k f + f^2) z^-2.
func inputSections(fc, rate float64) [2]sos {
	f := roundedTan(enginePi*fc/rate, Float64)
	f2 := f * f
	var out [2]sos
	for i, k := range []float64{-2.0 * roundedCos(5.0*enginePi/8.0), -2.0 * roundedCos(7.0*enginePi/8.0)} {
		a0 := 1 + k*f + f2
		out[i] = sos{
			b0: f2 / a0,
			b1: 2 * f2 / a0,
			b2: f2 / a0,
			a1: (2*f2 - 2) / a0,
			a2: (1 - k*f + f2) / a0,
		}
	}
	return out
}

// block runs ButterworthLowPassFilter::fast_f over x into y, which may be
// x, with the state in local variables. It sums the terms of the older
// outputs first, which shortens the dependency from one output to the
// next, and so rounds differently from engine-sim.
func (b *bw4) block(x, y []float32) {
	g, a1, a2, a3, a4 := b.g, b.a1, b.a2, b.a3, b.a4
	x1, x2, x3, x4 := b.x1, b.x2, b.x3, b.x4
	y1, y2, y3, y4 := b.y1, b.y2, b.y3, b.y4
	y = y[:len(x)]
	for i, v := range x {
		sum := v + 4*x1 + 6*x2 + 4*x3 + x4
		t := g*sum - (a2*y2 + a3*y3 + a4*y4)
		out := t - a1*y1
		x4, x3, x2, x1 = x3, x2, x1, v
		y4, y3, y2, y1 = y3, y2, y1, out
		y[i] = out
	}
	b.x1, b.x2, b.x3, b.x4 = x1, x2, x3, x4
	b.y1, b.y2, b.y3, b.y4 = y1, y2, y3, y4
}

// rtJitter is JitterFilter in float. The caller draws the positions and
// runs them through lp.
type rtJitter struct {
	jitterHistory
	gen minstdRand0
	lp  bw4
}

// jitterHistory is the sample history of a JitterFilter.
type jitterHistory struct {
	history [jitterTaps]float32
	offset  int
}

// f ports JitterFilter::fast_f after the noise filter: s is the filtered
// position. The floor and the ceiling avoid the math package.
func (j *jitterHistory) f(sample, s float32) float32 {
	j.history[j.offset] = sample
	j.offset++
	if j.offset >= jitterTaps {
		j.offset = 0
	}

	const top = jitterTaps - 1
	var i0, i1 int
	var si0 float32
	switch {
	case s <= 0:
		i0, i1, si0 = 0, 0, 0
	case s >= top:
		i0, i1, si0 = top, top, top
	default:
		// int truncates, which is the floor of a positive position.
		i0 = int(s)
		si0 = float32(i0)
		i1 = i0
		if si0 != s {
			i1++
		}
	}
	frac := s - si0

	i0 += j.offset
	i1 += j.offset
	if i0 >= jitterTaps {
		i0 -= jitterTaps
	}
	if i1 >= jitterTaps {
		i1 -= jitterTaps
	}
	return j.history[i1]*frac + j.history[i0]*(1-frac)
}

// uniformFast is uniformFloat32(0, b) without division: the modulus
// 2^31 - 1 folds the high bits, and 1/2^24 is exact.
func (g *minstdRand0) uniformFast(b float32) float32 {
	for {
		p := g.x * minstdMultiplier
		x := (p & minstdModulus) + (p >> 31)
		if x >= minstdModulus {
			x -= minstdModulus
		}
		g.x = x
		ret := float32((x-1)/127) * 0x1p-24
		if ret < 1 {
			return float32(ret * b)
		}
	}
}

// rtLeveller is LevelingFilter in float. It keeps the reciprocal of the
// peak: the decay multiplies it by 1/0.999, and a new peak computes it by
// Newton's method.
type rtLeveller struct {
	peak, invPeak, attenuation float32
	target, minLevel, maxLevel float32
}

const (
	levelDecay    = float32(0.999)
	levelInvDecay = float32(1 / 0.999)
)

var posInf32 = float32(math.Inf(1))

func (l *rtLeveller) f(sample float32) float32 {
	l.peak *= levelDecay
	l.invPeak *= levelInvDecay
	if l.peak < float32Min {
		l.peak = 0
	}
	a := sample
	if a < 0 {
		a = -a
	}
	if a > l.peak {
		l.peak = a
		l.invPeak = reciprocal(a)
	}
	if l.peak == 0 {
		return 0
	}
	att := l.target * l.invPeak
	if att < l.minLevel {
		att = l.minLevel
	} else if att > l.maxLevel {
		att = l.maxLevel
	}
	l.attenuation = 0.9*l.attenuation + 0.1*att
	return sample * l.attenuation
}

// reciprocal returns 1/a for a positive float32 by three Newton steps from
// an estimate of the exponent bits. It is +Inf below the smallest normal
// float, as 1/a overflows there.
func reciprocal(a float32) float32 {
	if a < float32Min {
		return posInf32
	}
	y := math.Float32frombits(0x7EF311C7 - math.Float32bits(a))
	y *= 2 - a*y
	y *= 2 - a*y
	y *= 2 - a*y
	return y
}
