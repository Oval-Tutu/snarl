package crackle

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// maxShocks is the number of shock envelopes that can sound at once. A
	// shock that starts when every slot is busy takes the slot of the
	// weakest.
	maxShocks = 32
	// shockEnd is the part of its peak at which a decaying shock envelope
	// ends, -80 dB.
	shockEnd = 1e-4
	// voiceEnd is the fade gain at which the voice ends, -100 dB.
	voiceEnd = 1e-5
	// nyquistShare is the part of the sample rate above which a band of a
	// bank has no gain.
	nyquistShare = 0.45
)

// butterQ are the Q values of the four stages of the eighth-order
// Butterworth low pass at the top of the banks.
var butterQ = [4]float64{0.5098, 0.6013, 0.9000, 2.5629}

// Options set one train.
type Options struct {
	// LevelDB is the gain of the train over the fitted level.
	LevelDB float64
	// Shocks is the count of shocks, clamped to the measured range, or 0
	// for a train of a length.
	Shocks int
	// LengthMS is the active length, clamped to the measured range, or 0
	// to draw it from the measured distribution.
	LengthMS float64
	// Single plays one shock at the level of the strongest shock of a
	// train, with the body and the bed until the fade at the end. It is
	// derived from the measured strong shocks of the trains: the clips
	// hold no isolated shock.
	Single bool
}

// shockEnv is the envelope of one shock: a linear rise by step to peak,
// then a fast decay of the crack e and a slow decay of the ring r.
type shockEnv struct {
	e, r, peak, step float32
	rising           bool
}

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
	var top [len(butterQ)]dsp.Biquad
	for i := range top {
		top[i].SetLowPass(topHz, butterQ[i], rate)
	}
	for i := range out {
		y := f.process(c.next())
		for k := range top {
			y = top[k].Process(y)
		}
		out[i] = float64(y)
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

// Voice plays one crackle train at a time. Process adds it to a block.
// The output for a seed and a sequence of calls is the same on every run.
type Voice struct {
	p    *Params
	rate float64

	rng, bodyNoise                        dsp.Noise
	shockCarrier, ringCarrier, bedCarrier [Bands]carrier
	shockBank, ringBank, bedBank          [Bands]bandFilter
	shockGain, ringGain, bedGain          [Bands]float32
	top                                   [len(butterQ)]dsp.Biquad
	body                                  bandFilter
	bodyGain                              float32
	attack, decay, ring                   float32
	fadeCoef, fadeStep                    float32
	// The tail is a damped sine at the end of the train: tailRe and
	// tailIm turn by tailTurn each sample.
	tailTurn       complex64
	tailRe, tailIm float32
	tailGain       float32

	envs [maxShocks]shockEnv
	nEnv int

	active bool
	single bool
	t      int64
	next   int64 // Sample of the next shock, -1 for none.
	left   int   // Shocks left, or -1 for a train of a length.
	end    int64 // Last sample at which a shock of a train of a length can start.
	fadeAt int64
	count  int
	total  float64 // Shocks or samples of the span, for the place offset.
	gain   float32
	z      float64 // Normal value of the level of the last shock.
	fade   float32
	in     float32
}

// New returns a voice with the parameters at the sample rate in hertz.
func New(p *Params, rate float64, seed uint32) *Voice {
	v := &Voice{
		p: p, rate: rate,
		rng:       dsp.NewNoise(seed),
		bodyNoise: dsp.NewNoise(seed ^ 0x165667b1),
		next:      -1,
	}
	for b := range Bands {
		v.shockCarrier[b].set(b, rate, seed^uint32(0x5bd1e995*(b+1)))
		v.ringCarrier[b].set(b, rate, seed^uint32(0x2545f491*(b+1)))
		v.bedCarrier[b].set(b, rate, seed^uint32(0x27d4eb2f*(b+1)))
		if v.shockBank[b].set(b, rate) && v.ringBank[b].set(b, rate) && v.bedBank[b].set(b, rate) {
			v.shockGain[b] = float32(dbAmp(p.ShockDB[b]))
			v.ringGain[b] = float32(dbAmp(p.RingDB[b]))
			v.bedGain[b] = float32(dbAmp(p.BedDB[b]))
		}
	}
	for i := range v.top {
		v.top[i].SetLowPass(p.TopHz, butterQ[i], rate)
	}
	v.body.setAt(p.BodyHz, p.BodyQ, rate)
	v.bodyGain = float32(dbAmp(p.BodyGainDB))
	v.attack = float32(max(1, p.AttackMS/1000*rate))
	v.decay = float32(math.Exp(-1 / (p.DecayMS / 1000 * rate)))
	v.ring = float32(math.Exp(-1 / (p.RingMS / 1000 * rate)))
	v.fadeCoef = float32(dbAmp(-p.FadeDBPerMS * 1000 / rate))
	v.fadeStep = 1
	if p.FadeInMS > 0 {
		v.fadeStep = float32(1 / (p.FadeInMS / 1000 * rate))
	}
	// The tail falls 20 dB in TailDecayMS.
	r := dbAmp(-20 / (p.TailDecayMS / 1000 * rate))
	s, c := math.Sincos(2 * math.Pi * p.TailHz / rate)
	v.tailTurn = complex(float32(r*c), float32(r*s))
	v.tailGain = float32(dbAmp(p.TailGainDB))
	return v
}

// Active reports whether a train sounds.
func (v *Voice) Active() bool { return v.active }

func (v *Voice) samples(ms float64) int64 { return int64(math.Round(ms / 1000 * v.rate)) }

// draw returns a value of a table of the values at evenly spaced
// probabilities from 0 to 1, at a uniform random probability.
func (v *Voice) draw(t []float64) float64 {
	return lerp(t, float64(v.rng.Unipolar())*float64(len(t)-1))
}

// drawMid returns a value of a quantile table at a uniform random
// probability. Value i of the table is at the probability (i + 0.5) / n,
// and the probabilities beyond the first and the last take their values.
func (v *Voice) drawMid(t []float64) float64 { return midAt(t, float64(v.rng.Unipolar())) }

// midAt returns the value of a quantile table at the probability u.
func midAt(t []float64, u float64) float64 {
	u = u*float64(len(t)) - 0.5
	return lerp(t, min(max(u, 0), float64(len(t)-1)))
}

// gap draws the gap to the next shock in samples.
func (v *Voice) gap() int64 { return v.samples(v.p.GapScale * v.drawMid(v.p.GapMS[:])) }

// gauss returns a normal value with the Box-Muller transform.
func (v *Voice) gauss() float64 {
	u1 := (float64(v.rng.Next()>>8) + 1) / (1 << 24)
	u2 := float64(v.rng.Unipolar())
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// level draws the level of a shock in dB, with the spread of the table
// around its top value scaled by LevelScale. The probability of the level
// is that of a normal value that follows the value of the shock before
// with the correlation LevelCorr, so that loud shocks come in groups.
func (v *Voice) level() float64 {
	r := v.p.LevelCorr
	v.z = r*v.z + math.Sqrt(1-r*r)*v.gauss()
	u := 0.5 * (1 + math.Erf(v.z/math.Sqrt2))
	top := v.p.LevelDB[Quantiles-1]
	return top + v.p.LevelScale*(midAt(v.p.LevelDB[:], u)-top)
}

// lerp returns the value of t at the place u, from 0 to len(t) - 1, with
// linear interpolation.
func lerp(t []float64, u float64) float64 {
	i := min(int(u), len(t)-2)
	return t[i] + (u-float64(i))*(t[i+1]-t[i])
}

// Trigger starts a train with a shock at its first sample. A train that
// sounds stops.
func (v *Voice) Trigger(o Options) {
	p := v.p
	v.active, v.single = true, o.Single
	v.t, v.count, v.nEnv, v.next = 0, 0, 0, 0
	v.fadeAt = math.MaxInt64
	v.fade, v.in = 1, min(1, v.fadeStep)
	v.tailRe, v.tailIm = 0, 0
	v.z = v.gauss()
	v.gain = float32(dbAmp(p.OutDB + o.LevelDB))
	// The bed and the body run before the first sample, so that the train
	// starts under way.
	for range v.samples(p.PrerollMS) {
		var y float32
		for b := range Bands {
			y += v.bedGain[b] * v.bedBank[b].process(v.bedCarrier[b].next())
		}
		for k := range v.top {
			y = v.top[k].Process(y)
		}
		v.body.process(v.bodyNoise.Bipolar())
	}
	switch {
	case o.Single:
		v.left, v.total = 1, 1
	case o.Shocks > 0:
		n := min(max(o.Shocks, p.ShocksMin), p.ShocksMax)
		v.left, v.total = n, float64(n)
	default:
		l := o.LengthMS
		if l > 0 {
			l = min(max(l, p.LengthMS[0]), p.LengthMS[4])
		} else {
			l = v.draw(p.LengthMS[:])
		}
		v.left = -1
		v.end = max(1, v.samples(l-p.LengthOffsetMS))
		v.total = float64(v.end)
	}
}

// fire starts a shock and schedules the next one.
func (v *Voice) fire() {
	p := v.p
	db := p.LevelDB[Quantiles-1]
	if !v.single {
		var place float64
		if v.left < 0 {
			place = float64(v.t) / v.total
		} else {
			place = float64(v.count) / v.total
		}
		db = v.level() + p.PlaceDB[min(9, int(place*10))]
	}
	peak := float32(dbAmp(db))
	k := v.nEnv
	if k == maxShocks {
		k = 0
		for i := range v.envs {
			if v.envs[i].e+v.envs[i].r < v.envs[k].e+v.envs[k].r {
				k = i
			}
		}
	} else {
		v.nEnv++
	}
	v.envs[k] = shockEnv{peak: peak, step: peak / v.attack, rising: true}
	v.count++

	if v.left > 0 {
		v.left--
	}
	gap := v.gap()
	if v.left == 0 || (v.left < 0 && v.t+gap > v.end) {
		v.next = -1
		v.fadeAt = v.t + v.samples(p.FadeDelayMS)
		return
	}
	v.next = v.t + max(1, gap)
}

// Process adds the train to out, one sample per value. It does not
// allocate.
func (v *Voice) Process(out []float32) {
	if !v.active {
		return
	}
	for i := range out {
		if v.t == v.next {
			v.fire()
		}
		var crack, ring float32
		for k := 0; k < v.nEnv; {
			e := &v.envs[k]
			if e.rising {
				if e.e += e.step; e.e >= e.peak {
					e.e, e.r, e.rising = e.peak, e.peak, false
				}
			} else {
				e.e *= v.decay
				if e.r *= v.ring; e.r < e.peak*shockEnd {
					v.nEnv--
					v.envs[k] = v.envs[v.nEnv]
					continue
				}
			}
			crack += e.e
			ring += e.r
			k++
		}
		// Each band has its own carrier, so that the band powers add.
		var y float32
		for b := range Bands {
			y += v.shockGain[b]*v.shockBank[b].process(v.shockCarrier[b].next()*crack) +
				v.ringGain[b]*v.ringBank[b].process(v.ringCarrier[b].next()*ring) +
				v.bedGain[b]*v.bedBank[b].process(v.bedCarrier[b].next())
		}
		for k := range v.top {
			y = v.top[k].Process(y)
		}
		// At the fade the body stops and rings out, and the tail starts.
		var body float32
		if v.t < v.fadeAt {
			body = v.bodyNoise.Bipolar()
		} else if v.t == v.fadeAt {
			v.tailRe = v.tailGain
		}
		y += v.bodyGain * v.body.process(body)
		if v.t >= v.fadeAt {
			if v.fade *= v.fadeCoef; v.fade < voiceEnd {
				v.active = false
				return
			}
		}
		z := complex(v.tailRe, v.tailIm) * v.tailTurn
		v.tailRe, v.tailIm = real(z), imag(z)
		out[i] += (y*v.fade*v.in + v.tailIm) * v.gain
		v.in = min(1, v.in+v.fadeStep)
		v.t++
	}
}

// Render returns one train at the sample rate, from its start to its end.
func Render(p *Params, rate float64, seed uint32, o Options) []float32 {
	v := New(p, rate, seed)
	v.Trigger(o)
	var out []float32
	buf := make([]float32, 256)
	for v.Active() {
		clear(buf)
		v.Process(buf)
		out = append(out, buf...)
	}
	return out
}

func dbAmp(db float64) float64 { return math.Pow(10, db/20) }
