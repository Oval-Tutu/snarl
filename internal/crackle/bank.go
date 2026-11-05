package crackle

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// grainCount is the number of shock grains of a bank. Each shock plays
	// one at random, with a random sign.
	grainCount = 64
	// grainEnd is the part of its peak at which the ring of a grain ends,
	// -60 dB, and maxGrainS the longest grain in seconds.
	grainEnd  = 1e-3
	maxGrainS = 0.2
	// grainFadeS is the linear fade at the end of a grain against a click.
	grainFadeS = 0.001
	// loopS is the length of the loops of the bed, the ring and the body,
	// loopFadeS the cross fade that joins the end of a loop to its start,
	// and warmS the time for which the filters run before a loop starts.
	loopS     = 2
	loopFadeS = 0.05
	warmS     = 0.1
	// shockEnd is the part of its peak at which a decaying shock envelope
	// ends in the envelope energy, -80 dB.
	shockEnd = 1e-4
	// nyquistShare is the part of the sample rate above which a band of a
	// bank has no gain.
	nyquistShare = 0.45
)

// butterQ are the Q values of the four stages of the eighth-order
// Butterworth low pass at the top of the banks.
var butterQ = [4]float64{0.5098, 0.6013, 0.9000, 2.5629}

// bandFilter is the filter of one band of a bank: two equal band pass
// stages, whose half-power points are the band edges.
type bandFilter struct{ a, b dsp.Biquad }

func (f *bandFilter) process(x float32) float32 { return f.b.Process(f.a.Process(x)) }

// set designs the filter of band b at the rate. It reports false for a
// band too close to the Nyquist frequency, which has no gain.
func (f *bandFilter) set(b int, rate float64) bool {
	e := BandEdges[b]
	fc := math.Sqrt(e[0] * e[1])
	if fc >= nyquistShare*rate {
		return false
	}
	f.setAt(fc, q2(e[1]/fc-fc/e[1]), rate)
	return true
}

func (f *bandFilter) setAt(fc, q, rate float64) {
	f.a.SetBandPass(fc, q, rate)
	f.b.SetBandPass(fc, q, rate)
}

// q2 returns the Q of each of two equal band pass stages whose product is
// at half power at x = f/fc - fc/f: each stage has |H|^2 = 1 / (1 + q^2
// x^2), so the two are at half power where q^2 x^2 = sqrt(2) - 1.
func q2(x float64) float64 { return math.Sqrt(math.Sqrt2-1) / x }

// impulse returns the first n samples of the impulse response of process.
func impulse(process func(float32) float32, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := float32(0)
		if i == 0 {
			x = 1
		}
		out[i] = float64(process(x))
	}
	return out
}

// carrier is a sine at the centre of a band whose phase takes a random
// walk, so that it has the spectrum of a narrow band of noise around the
// centre and a constant envelope. A noise of the same band has an envelope
// that rises and falls, which the shock detector of the measures takes
// for more shocks.
type carrier struct {
	noise           dsp.Noise
	phase, inc, wlk float32
}

// carrierAmp is the amplitude of a carrier, whose power is NoisePower.
var carrierAmp = float32(math.Sqrt(2 * NoisePower))

// set tunes the carrier to band b at the rate. The random walk of the
// phase gives a line whose width at half power is half the band: a step
// uniform from -w to w cycles has the variance w^2 / 3, and a phase walk
// with the variance D in rad^2 per second has the width D / (2 pi) Hz.
func (c *carrier) set(b int, rate float64, seed uint32) {
	e := BandEdges[b]
	width := (e[1] - e[0]) / 2
	c.noise = dsp.NewNoise(seed)
	c.inc = float32(math.Sqrt(e[0]*e[1]) / rate)
	c.wlk = float32(math.Sqrt(3 * width / (2 * math.Pi * rate)))
}

func (c *carrier) next() float32 {
	p := c.phase + c.inc + c.wlk*c.noise.Bipolar()
	if p >= 1 {
		p--
	} else if p < 0 {
		p++
	}
	c.phase = p
	return carrierAmp * dsp.Sine(p)
}

// top is the low pass at the top of the banks.
type top [len(butterQ)]dsp.Biquad

func newTop(hz, rate float64) *top {
	var t top
	for i := range t {
		t[i].SetLowPass(hz, butterQ[i], rate)
	}
	return &t
}

func (t *top) process(y float32) float32 {
	for i := range t {
		y = t[i].Process(y)
	}
	return y
}

// bank is one bank of octave band filters with a carrier in each band.
type bank struct {
	c    [Bands]carrier
	f    [Bands]bandFilter
	gain [Bands]float32
}

func newBank(db *[Bands]float64, rate float64, seed, mix uint32) *bank {
	k := &bank{}
	for b := range Bands {
		k.c[b].set(b, rate, seed^mix*uint32(b+1))
		if k.f[b].set(b, rate) {
			k.gain[b] = float32(dbAmp(db[b]))
		}
	}
	return k
}

// next returns the next sample of the bank with the carriers at the
// amplitude env.
func (k *bank) next(env float32) float32 {
	var y float32
	for b := range Bands {
		y += k.gain[b] * k.f[b].process(k.c[b].next()*env)
	}
	return y
}

// BankSignal returns n samples of the carrier of band b through the filter
// of band b and the low pass at the top at topHz, at the rate, with the
// gain 1, for the fit of the gains.
func BankSignal(b int, topHz, rate float64, n int) []float64 {
	var f bandFilter
	out := make([]float64, n)
	if !f.set(b, rate) {
		return out
	}
	var c carrier
	c.set(b, rate, uint32(b+1))
	t := newTop(topHz, rate)
	for i := range out {
		out[i] = float64(t.process(f.process(c.next())))
	}
	return out
}

// BodyResponse returns the first n samples of the impulse response of the
// body filter at hz with q at the rate: two equal band pass stages.
func BodyResponse(hz, q, rate float64, n int) []float64 {
	var f bandFilter
	f.setAt(hz, q, rate)
	return impulse(f.process, n)
}

// NoisePower is the power of the noise of the body, uniform from -1 to 1,
// and of the carriers of the banks.
const NoisePower = 1.0 / 3

// EnvelopeEnergy returns the sums of the squared envelopes of the crack
// and of the ring of a shock at 0 dB at the rate.
func (p *Params) EnvelopeEnergy(rate float64) (crack, ring float64) {
	attack := max(1, p.AttackMS/1000*rate)
	for e := 1 / attack; e < 1; e += 1 / attack {
		crack += e * e
	}
	for e, d := 1.0, math.Exp(-1/(p.DecayMS/1000*rate)); e >= shockEnd; e *= d {
		crack += e * e
	}
	for e, d := 1.0, math.Exp(-1/(p.RingMS/1000*rate)); e >= shockEnd; e *= d {
		ring += e * e
	}
	return crack, ring
}

// Bank holds the sound of the model at one sample rate, which NewBank
// renders once from the parameters: a table of shock grains, and loops of
// the bed, the ring and the body. Voices only read a bank, so many voices
// can share one.
type Bank struct {
	p    *Params
	rate float64
	// grains holds the shocks at 0 dB: the crack and the ring through
	// their banks and the low pass at the top.
	grains [][]float32
	// bed, ring and body are loops of the same length: the bed bank, the
	// ring bank with its carriers at full amplitude, and the body.
	bed, ring, body []float32
}

// ShortTail returns a bank with the grains and the loops of b and the
// parameters of b with the short tail (Params.ShortTail).
func (b *Bank) ShortTail() *Bank {
	c := *b
	c.p = b.p.ShortTail()
	return &c
}

// DrawnLevels returns a bank with the grains and the loops of b and the
// parameters of b with the levels of a short train as drawn, with no move
// to its strongest shock (Options.Short).
func (b *Bank) DrawnLevels() *Bank {
	c, p := *b, *b.p
	p.drawnLevels = true
	c.p = &p
	return &c
}

// NewBank renders the sound of the model at the sample rate in hertz. The
// seed sets the carriers and the noise of the grains and the loops.
func NewBank(p *Params, rate float64, seed uint32) *Bank {
	k := &Bank{p: p, rate: rate}
	for g := range grainCount {
		k.grains = append(k.grains, k.grain(seed+uint32(g)*0x9e3779b9))
	}
	n, fade, warm := int(loopS*rate), int(loopFadeS*rate), int(warmS*rate)
	bed := newBank(&p.BedDB, rate, seed, 0x27d4eb2f)
	ring := newBank(&p.RingDB, rate, seed, 0x2545f491)
	tb, tr := newTop(p.TopHz, rate), newTop(p.TopHz, rate)
	k.bed = loop(func() float32 { return tb.process(bed.next(1)) }, n, fade, warm)
	k.ring = loop(func() float32 { return tr.process(ring.next(1)) }, n, fade, warm)
	var body bandFilter
	body.setAt(p.BodyHz, p.BodyQ, rate)
	noise := dsp.NewNoise(seed ^ 0x165667b1)
	gain := float32(dbAmp(p.BodyGainDB))
	k.body = loop(func() float32 { return gain * body.process(noise.Bipolar()) }, n, fade, warm)
	return k
}

// grain renders one shock at 0 dB: a linear rise, then the crack with a
// fast decay and the ring with a slow decay, until the ring falls to
// grainEnd.
func (k *Bank) grain(seed uint32) []float32 {
	p, rate := k.p, k.rate
	crack := newBank(&p.ShockDB, rate, seed, 0x5bd1e995)
	ring := newBank(&p.RingDB, rate, seed, 0x2545f491)
	t := newTop(p.TopHz, rate)
	attack := max(1, p.AttackMS/1000*rate)
	n := min(int(maxGrainS*rate), int(attack+p.RingMS/1000*rate*math.Log(1/grainEnd)))
	decay, rd := float32(math.Exp(-1/(p.DecayMS/1000*rate))), float32(math.Exp(-1/(p.RingMS/1000*rate)))
	out := make([]float32, n)
	var e, r float32
	rising := true
	for i := range out {
		if rising {
			if e += float32(1 / attack); e >= 1 {
				e, r, rising = 1, 1, false
			}
		} else {
			e, r = e*decay, r*rd
		}
		out[i] = t.process(crack.next(e) + ring.next(r))
	}
	f := min(n, int(grainFadeS*rate))
	for i := range f {
		out[n-1-i] *= float32(i) / float32(f)
	}
	return out
}

// loop returns n samples of next after warm samples, with the f samples
// after the n joined to the start by an equal-power cross fade, so that
// the loop has no step where it wraps.
func loop(next func() float32, n, f, warm int) []float32 {
	for range warm {
		next()
	}
	x := make([]float32, n+f)
	for i := range x {
		x[i] = next()
	}
	for i := range f {
		s, c := math.Sincos(math.Pi / 2 * float64(i) / float64(f))
		x[i] = x[i]*float32(s) + x[n+i]*float32(c)
	}
	return x[:n]
}
