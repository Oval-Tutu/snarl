package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// path is one exhaust path from a bank collector to an outlet. The linear
// stages run twice while a pop sounds: once for the engine and the pop,
// and once for the pop alone, so that the steepening acts on the engine
// wave only and the pop passes it unchanged.
type path struct {
	eng, pop  linear
	colD      int
	colG      float32
	chD       int
	chR2, chT float32
	steep     float32

	outlet       dsp.Biquad
	direct, gain float32
	air          *dsp.Delay
	airD         int
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
		colD:   colD,
		colG:   float32(p.Reflection),
		direct: float32(e.OutletDirect),
		gain:   float32(p.Gain),
	}
	l := &q.eng
	l.col = dsp.NewDelay(colD)
	l.colLP = dsp.OnePole{A: dsp.OnePoleCoef(p.DampingHz, rate)}
	if p.DampingHz <= 0 {
		l.colLP.A = 1
	}
	q.outlet.SetOutlet(p.OutletM, c, rate)
	if d := int(math.Round(p.ListenerM / 343 * rate)); d > 0 {
		q.air = dsp.NewDelay(d)
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

// linear runs the linear stages of l for one sample: the collector
// resonance, the wall-loss shelf, the low shelf, the expansion chamber and
// the mode cutoff.
func (p *path) linear(l *linear, x float32) float32 {
	y := x + p.colG*l.colLP.Process(l.col.Read(p.colD-1))
	l.col.Write(y)
	y = l.low.Process(l.wall.Process(y))
	if l.ch != nil {
		w := y + p.chR2*l.ch.Read(2*p.chD-1)
		l.ch.Write(w)
		y = p.chT * l.ch.Read(p.chD)
	}
	return l.mode2.Process(l.mode1.Process(y))
}

// process runs the path for one sample of the engine wave x and the pop
// jump j: the linear stages, the wave steepening of the engine wave, the
// outlet filter and the air delay to the listener. While pop is true, the
// pop stages run, so that the steepening leaves out the pop.
func (p *path) process(x, j float32, pop bool) float32 {
	// The engine stages are written out here, so that they need no call.
	l := &p.eng
	y := x + j + p.colG*l.colLP.Process(l.col.Read(p.colD-1))
	l.col.Write(y)
	y = l.low.Process(l.wall.Process(y))
	if l.ch != nil {
		w := y + p.chR2*l.ch.Read(2*p.chD-1)
		l.ch.Write(w)
		y = p.chT * l.ch.Read(p.chD)
	}
	y = l.mode2.Process(l.mode1.Process(y))
	w := y
	if pop {
		w -= p.linear(&p.pop, j)
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
		return y
	}
	p.air.Write(y)
	return p.air.Read(p.airD)
}
