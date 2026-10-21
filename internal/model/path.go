package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// popTailSec is the time in seconds for which the pop stages of a path run
// after the pop jump of the path ends, so that the linear stages ring out.
const popTailSec = 0.1

// path is one exhaust path from a bank collector to an outlet. The linear
// stages run twice while a pop of the path sounds: once for the engine and
// the pop, and once for the pop alone, so that the steepening acts on the
// engine wave only and the pop passes it unchanged.
type path struct {
	eng, pop  linear
	colD      int
	colG      float32
	chD       int
	chR2, chT float32
	steep     float32
	// popLeft is the samples left in which the pop stages run. Each sample
	// with a live pop jump sets it to popTail. When it reaches 0, the pop
	// stages clear, so that the next pop starts from silence.
	popLeft, popTail int

	// outlet and air are the outlet filter and the air delay of the
	// engine wave. popOutlet and popAir are the same stages for the pop
	// alone, which run with the pop stages, so that the pops pass the
	// leveller of the output stage unchanged.
	outlet, popOutlet dsp.Biquad
	direct, gain      float32
	air, popAir       *dsp.Delay
	airD              int
}

// linear holds the state of the linear stages of a path: the collector,
// the wall-loss and low shelves, the expansion chamber and the mode cutoff.
type linear struct {
	col          *dsp.Delay
	colLP        dsp.OnePole
	low, wall    dsp.Shelf
	ch           *dsp.Delay
	mode1, mode2 dsp.Biquad
}

func newPath(p PathSpec, e ExhaustSpec, rate float64) path {
	c := e.SpeedOfSound
	colD := max(1, int(math.Round(2*p.CollectorM/c*rate)))
	q := path{
		colD:    colD,
		colG:    float32(p.Reflection),
		direct:  float32(e.OutletDirect),
		gain:    float32(p.Gain),
		popTail: max(1, int(popTailSec*rate)),
	}
	l := &q.eng
	l.col = dsp.NewDelay(colD)
	l.colLP = dsp.OnePole{A: dsp.OnePoleCoef(p.DampingHz, rate)}
	if p.DampingHz <= 0 {
		l.colLP.A = 1
	}
	q.outlet.SetOutlet(p.OutletM, c, rate)
	q.popOutlet = q.outlet
	if d := int(math.Round(p.ListenerM / 343 * rate)); d > 0 {
		q.air = dsp.NewDelay(d)
		q.popAir = dsp.NewDelay(d)
		q.airD = d
	}

	wallHz := p.WallLossHz
	if wallHz <= 0 {
		wallHz = 1500
	}
	l.wall = dsp.NewHighShelf(wallHz, math.Pow(10, -p.WallLossDB/20), rate)
	l.low = dsp.NewLowShelf(e.LowShelfHz, math.Pow(10, e.LowShelfDB/20), rate)
	if p.ChamberM > 0 && p.ChamberArea > 1 {
		q.chD = max(1, int(math.Round(p.ChamberM/c*rate)))
		l.ch = dsp.NewDelay(2 * q.chD)
		r := (p.ChamberArea - 1) / (p.ChamberArea + 1)
		q.chR2 = float32(r * r * 0.97)
		q.chT = float32(1 - r*r)
	}
	// Plane-mode cutoff, a fourth-order Butterworth low pass.
	fc := 1.8412 * c / (2 * math.Pi * p.PipeRadiusM)
	l.mode1.SetLowPass(fc, 0.5412, rate)
	l.mode2.SetLowPass(fc, 1.3066, rate)
	q.pop = *l
	q.pop.col = dsp.NewDelay(colD)
	if l.ch != nil {
		q.pop.ch = dsp.NewDelay(2 * q.chD)
	}
	return q
}

// reset clears the state of the linear stages.
func (l *linear) reset() {
	l.col.Reset()
	l.colLP.Y = 0
	l.low.Reset()
	l.wall.Reset()
	if l.ch != nil {
		l.ch.Reset()
	}
	l.mode1.Reset()
	l.mode2.Reset()
}

// process runs the path for one sample of the engine wave x and the pop
// jump j: the linear stages, the wave steepening of the engine wave, the
// outlet filter and the air delay to the listener. It returns the engine
// output and the pop output. live is true while the pop jump of the path
// sounds. The pop stages run from then until popTail samples after the
// last live sample, so that the steepening and the leveller leave out the
// pop. Outside the pop stages, the pop output is zero and a pop jump in j
// is part of the engine output.
func (p *path) process(x, j float32, live bool) (eng, pop float32) {
	if live {
		p.popLeft = p.popTail
	}
	var y, w float32
	if p.popLeft > 0 {
		y, w = p.linear2(x+j, j)
		v := y - w
		pop = p.gain * (p.direct*v + p.popOutlet.Process(v))
		if p.popAir != nil {
			p.popAir.Write(pop)
			pop = p.popAir.Read(p.airD)
		}
		y = w
		p.popLeft--
		if p.popLeft == 0 {
			p.pop.reset()
			p.popOutlet.Reset()
			if p.popAir != nil {
				p.popAir.Reset()
			}
		}
	} else {
		// The engine stages are written out here, so that they need no call.
		l := &p.eng
		y = x + j + p.colG*l.colLP.Process(l.col.Read(p.colD-1))
		l.col.Write(y)
		y = l.low.Process(l.wall.Process(y))
		if l.ch != nil {
			v := y + p.chR2*l.ch.Read(2*p.chD-1)
			l.ch.Write(v)
			y = p.chT * l.ch.Read(p.chD)
		}
		y = l.mode2.Process(l.mode1.Process(y))
		w = y
	}
	a := w
	if a < 0 {
		a = -a
	}
	if a > 4 {
		a = 4
	}
	y += p.steep * w * a
	y = p.gain * (p.direct*y + p.outlet.Process(y))
	if p.air == nil {
		return y, pop
	}
	p.air.Write(y)
	return p.air.Read(p.airD), pop
}

// linear2 runs the linear stages for one sample of the engine and the pop
// x in the engine stages and of the pop j in the pop stages. It returns
// the output y of the engine stages and the engine wave w, which is y
// without the pop. The two sets of stages run side by side, so that the
// processor can overlap them.
func (p *path) linear2(x, j float32) (y, w float32) {
	e, q := &p.eng, &p.pop
	y = x + p.colG*e.colLP.Process(e.col.Read(p.colD-1))
	v := j + p.colG*q.colLP.Process(q.col.Read(p.colD-1))
	e.col.Write(y)
	q.col.Write(v)
	y = e.low.Process(e.wall.Process(y))
	v = q.low.Process(q.wall.Process(v))
	if e.ch != nil {
		ye := y + p.chR2*e.ch.Read(2*p.chD-1)
		yq := v + p.chR2*q.ch.Read(2*p.chD-1)
		e.ch.Write(ye)
		q.ch.Write(yq)
		y = p.chT * e.ch.Read(p.chD)
		v = p.chT * q.ch.Read(p.chD)
	}
	y = e.mode2.Process(e.mode1.Process(y))
	v = q.mode2.Process(q.mode1.Process(v))
	return y, y - v
}
