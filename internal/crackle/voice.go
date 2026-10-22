package crackle

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// maxShocks is the number of grains that can sound at once. A shock
	// that starts when every slot is busy takes the slot of the oldest.
	maxShocks = 32
	// voiceEnd is the fade gain at which the voice ends, -100 dB.
	voiceEnd = 1e-5
)

// Options set one train.
type Options struct {
	// LevelDB is the gain of the train over the fitted level.
	LevelDB float64
	// Shocks is the count of shocks, clamped to the measured range, or 0
	// for a train of a length.
	Shocks int
	// Short lets Shocks go below the measured minimum, down to 1. A short
	// train is derived, not measured: no clip holds one.
	Short bool
	// LengthMS is the active length, clamped to the measured range, or 0
	// to draw it from the measured distribution.
	LengthMS float64
	// Single plays one shock at the level of the strongest shock of a
	// train, with the body and the bed until the fade at the end. It is
	// derived from the measured strong shocks of the trains: the clips
	// hold no isolated shock.
	Single bool
}

// play is one grain that sounds: the grain, the next sample and the gain.
type play struct {
	g   []float32
	pos int
	lvl float32
}

// Voice plays one crackle train at a time from a bank. Process adds it to
// a block. The output for a bank, a seed and a sequence of calls is the
// same on every run.
type Voice struct {
	b    *Bank
	p    *Params
	rate float64
	rng  dsp.Noise

	plays [maxShocks]play
	nPlay int
	// at is the place in the loops of the bed, the ring and the body.
	at int
	// act follows the shock activity: each shock adds its amplitude, and
	// it decays by actDecay each sample. It plays the ring loop at actGain.
	act, actDecay, actGain float32
	fadeCoef, fadeStep     float32
	// The tail is a damped sine at the end of the train: tailRe and
	// tailIm turn by tailTurn each sample.
	tailTurn       complex64
	tailRe, tailIm float32
	tailGain       float32

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

// New returns a voice that plays from the bank.
func New(b *Bank, seed uint32) *Voice {
	p, rate := b.p, b.rate
	v := &Voice{b: b, p: p, rate: rate, rng: dsp.NewNoise(seed), next: -1}
	v.actDecay = float32(math.Exp(-1 / (p.ActMS / 1000 * rate)))
	v.actGain = float32(dbAmp(p.ActDB))
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
// around its top value scaled by LevelScale. The probability u of the
// level is that of a normal value that follows the value of the shock
// before with the correlation LevelCorr, so that loud shocks come in
// groups. The place offset adds in full to the weakest level, and to a
// level at the probability u less PlaceWeight times u, so that with a
// PlaceWeight of 1 it moves the median of a tenth and not its top.
func (v *Voice) level(place float64) float64 {
	r := v.p.LevelCorr
	v.z = r*v.z + math.Sqrt(1-r*r)*v.gauss()
	u := 0.5 * (1 + math.Erf(v.z/math.Sqrt2))
	top := v.p.LevelDB[Quantiles-1]
	return top + v.p.LevelScale*(midAt(v.p.LevelDB[:], u)-top) + place*(1-v.p.PlaceWeight*u)
}

// lerp returns the value of t at the place u, from 0 to len(t) - 1, with
// linear interpolation.
func lerp(t []float64, u float64) float64 {
	i := min(int(u), len(t)-2)
	return t[i] + (u-float64(i))*(t[i+1]-t[i])
}

// Trigger starts a train with a shock at its first sample, at a random
// place in the loops, so that the bed and the body are under way. A train
// that sounds stops.
func (v *Voice) Trigger(o Options) {
	p := v.p
	v.active, v.single = true, o.Single
	v.t, v.count, v.nPlay, v.next = 0, 0, 0, 0
	v.at = int(v.rng.Next() % uint32(len(v.b.bed)))
	v.act = 0
	v.fadeAt = math.MaxInt64
	v.fade, v.in = 1, min(1, v.fadeStep)
	v.tailRe, v.tailIm = 0, 0
	v.z = v.gauss()
	v.gain = float32(dbAmp(p.OutDB + o.LevelDB))
	switch {
	case o.Single:
		v.left, v.total = 1, 1
	case o.Shocks > 0:
		lo := p.ShocksMin
		if o.Short {
			lo = 1
		}
		n := min(max(o.Shocks, lo), p.ShocksMax)
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
		db = v.level(p.PlaceDB[min(9, int(place*10))])
	}
	peak := float32(dbAmp(db))
	v.act += peak
	k := v.nPlay
	if k == maxShocks {
		k = 0
		for i := range v.plays {
			if v.plays[i].pos > v.plays[k].pos {
				k = i
			}
		}
	} else {
		v.nPlay++
	}
	n := v.rng.Next()
	lvl := peak
	if n&1 != 0 {
		lvl = -peak
	}
	v.plays[k] = play{g: v.b.grains[(n>>1)%grainCount], lvl: lvl}
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
	b := v.b
	for i := range out {
		if v.t == v.next {
			v.fire()
		}
		var y float32
		for k := 0; k < v.nPlay; {
			pl := &v.plays[k]
			y += pl.lvl * pl.g[pl.pos]
			if pl.pos++; pl.pos == len(pl.g) {
				v.nPlay--
				v.plays[k] = v.plays[v.nPlay]
				continue
			}
			k++
		}
		y += b.bed[v.at] + v.actGain*v.act*b.ring[v.at] + b.body[v.at]
		v.act *= v.actDecay
		if v.at++; v.at == len(b.bed) {
			v.at = 0
		}
		if v.t >= v.fadeAt {
			if v.t == v.fadeAt {
				v.tailRe = v.tailGain
			}
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

// Render returns one train of the bank, from its start to its end.
func Render(b *Bank, seed uint32, o Options) []float32 {
	v := New(b, seed)
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
