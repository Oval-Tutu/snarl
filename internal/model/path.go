package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// popTailSec is the time in seconds for which the pop stages of a path run
// after the pop jump of the path ends, so that the pop bank rings out.
const popTailSec = 0.1

// path is one exhaust path from a bank collector to the formant bank: the
// collector, the path gain and the air delay to the listener. The pops of
// the path have stages of their own, which run only while a pop of the
// path sounds: a collector, the pop bank, the path gain and the pop delay
// with the air delay. The pops join the engine after the leveller.
type path struct {
	col, popCol     *dsp.Delay
	colLP, popColLP dsp.OnePole
	colD            int
	colG            float32
	gain            float32
	air, popAir     *dsp.Delay
	airD, popD      int
	pop             formants
	// popLeft is the samples left in which the pop stages run. Each sample
	// with a live pop jump sets it to popTail. When it reaches 0, the pop
	// stages clear, so that the next pop starts from silence.
	popLeft, popTail int
	// quiet counts the samples after the jump in which the output of the
	// pop stages stays below popQuiet.
	quiet int
}

const (
	// popQuiet is the pop output, 100 dB below full scale, below which a
	// pop has rung out, and popQuietLen the samples that it must stay
	// there.
	popQuiet    = 1e-5
	popQuietLen = 64
)

func newPath(p PathSpec, s *Spec, rate float64) path {
	colD := max(1, int(math.Round(2*p.CollectorM/s.Exhaust.SpeedOfSound*rate)))
	q := path{
		col:     dsp.NewDelay(colD),
		popCol:  dsp.NewDelay(colD),
		colLP:   dsp.OnePole{A: dsp.OnePoleCoef(p.DampingHz, rate)},
		colD:    colD,
		colG:    float32(p.Reflection),
		gain:    float32(p.Gain),
		pop:     newFormants(s.PopFormants, s.Exhaust.PopFormantGainDB, rate),
		popTail: max(1, int(popTailSec*rate)),
	}
	if p.DampingHz <= 0 {
		q.colLP.A = 1
	}
	q.popColLP = q.colLP
	if d := int(math.Round(p.ListenerM / 343 * rate)); d > 0 {
		q.air = dsp.NewDelay(d)
		q.airD = d
	}
	if d := q.airD + int(math.Round(s.Exhaust.PopDelayMS*1e-3*rate)); d > 0 {
		q.popAir = dsp.NewDelay(d)
		q.popD = d
	}
	return q
}

// process runs the path for one sample of the engine wave x: the
// collector, the path gain and the air delay.
func (p *path) process(x float32) float32 {
	y := x + p.colG*p.colLP.Process(p.col.Read(p.colD-1))
	p.col.Write(y)
	y *= p.gain
	if p.air == nil {
		return y
	}
	p.air.Write(y)
	return p.air.Read(p.airD)
}

// popProcess runs the pop stages for one sample of the pop jump j of the
// path and returns the pop output: popPre, then the pop bank of the path.
// The calibration and PopSignals use it. The engine runs popPre on each
// path and one pop bank on their sum, which gives the same output, since
// the pop bank is linear and time-invariant and the same on every path.
func (p *path) popProcess(j float32, live bool) float32 {
	y := p.popPre(j, live)
	if p.popLeft == 0 && !live {
		p.pop.reset()
	}
	return p.pop.process(y)
}

// popPre runs the pop stages before the pop bank for one sample of the pop
// jump j of the path: the collector, the path gain and the pop delay with
// the air delay. live is true while the pop jump of the path sounds. The
// stages run from then until the output has stayed below popQuiet for
// popQuietLen samples plus the pop delay, so that the delay line holds
// nothing above it either, and at most popTail samples after the last
// live sample. Outside them, the output is zero.
func (p *path) popPre(j float32, live bool) float32 {
	if live {
		p.popLeft = p.popTail
	}
	if p.popLeft == 0 {
		return 0
	}
	y := j + p.colG*p.popColLP.Process(p.popCol.Read(p.colD-1))
	p.popCol.Write(y)
	y *= p.gain
	if !live && y < popQuiet && y > -popQuiet {
		p.quiet++
	} else {
		p.quiet = 0
	}
	if p.quiet > p.popD+popQuietLen {
		p.popLeft = 1
	}
	if p.popAir != nil {
		p.popAir.Write(y)
		y = p.popAir.Read(p.popD)
	}
	p.popLeft--
	if p.popLeft == 0 {
		p.quiet = 0
		p.popCol.Reset()
		p.popColLP.Y = 0
		if p.popAir != nil {
			p.popAir.Reset()
		}
	}
	return y
}

// popBank is the pop bank that the engine runs once on the sum of the pop
// stages of its paths. It runs while a path runs its pop stages, and after
// that until its output has stayed below popQuiet for popQuietLen
// samples, when it clears, so that the next pop starts from silence.
type popBank struct {
	f     formants
	on    bool
	quiet int
}

// process filters the sum x of the pop stages of the paths. run is true
// while a path runs its pop stages.
func (b *popBank) process(x float32, run bool) float32 {
	if !run && !b.on {
		return 0
	}
	y := b.f.process(x)
	switch {
	case run:
		b.on, b.quiet = true, 0
	case y < popQuiet && y > -popQuiet:
		b.quiet++
		if b.quiet > popQuietLen {
			b.on, b.quiet = false, 0
			b.f.reset()
		}
	default:
		b.quiet = 0
	}
	return y
}
