package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// path is one exhaust path from a bank collector to an outlet.
type path struct {
	col   *dsp.Delay
	colD  int
	colG  float32
	colLP dsp.OnePole

	low          dsp.Shelf
	wall         dsp.Shelf
	ch           *dsp.Delay
	chD          int
	chR2, chT    float32
	mode1, mode2 dsp.Biquad
	steep        float32

	outlet       dsp.Biquad
	direct, gain float32
	air          *dsp.Delay
	airD         int
}

func newPath(p PathSpec, e ExhaustSpec, rate float64) path {
	c := e.SpeedOfSound
	colD := max(1, int(math.Round(2*p.CollectorM/c*rate)))
	q := path{
		col:    dsp.NewDelay(colD),
		colD:   colD,
		colG:   float32(p.Reflection),
		colLP:  dsp.OnePole{A: dsp.OnePoleCoef(p.DampingHz, rate)},
		direct: float32(e.OutletDirect),
		gain:   float32(p.Gain),
	}
	if p.DampingHz <= 0 {
		q.colLP.A = 1
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
	q.wall = dsp.NewHighShelf(wallHz, math.Pow(10, -p.WallLossDB/20), rate)
	q.low = dsp.NewLowShelf(e.LowShelfHz, math.Pow(10, e.LowShelfDB/20), rate)
	if p.ChamberM > 0 && p.ChamberArea > 1 {
		q.chD = max(1, int(math.Round(p.ChamberM/c*rate)))
		q.ch = dsp.NewDelay(2 * q.chD)
		r := (p.ChamberArea - 1) / (p.ChamberArea + 1)
		q.chR2 = float32(r * r * 0.97)
		q.chT = float32(1 - r*r)
	}
	// Plane-mode cutoff, a fourth-order Butterworth low pass.
	fc := 1.8412 * c / (2 * math.Pi * p.PipeRadiusM)
	q.mode1.SetLowPass(fc, 0.5412, rate)
	q.mode2.SetLowPass(fc, 1.3066, rate)
	return q
}

// process runs the path for one sample: the collector resonance, the
// wall-loss shelf, the low shelf, the expansion chamber, the mode cutoff,
// the wave steepening, the outlet filter and the air delay to the
// listener.
func (p *path) process(x float32) float32 {
	y := x + p.colG*p.colLP.Process(p.col.Read(p.colD-1))
	p.col.Write(y)
	y = p.low.Process(p.wall.Process(y))
	if p.ch != nil {
		w := y + p.chR2*p.ch.Read(2*p.chD-1)
		p.ch.Write(w)
		y = p.chT * p.ch.Read(p.chD)
	}
	y = p.mode2.Process(p.mode1.Process(y))
	a := y
	if a < 0 {
		a = -a
	}
	if a > 4 {
		a = 4
	}
	y += p.steep * y * a
	y = p.gain * (p.direct*y + p.outlet.Process(y))
	if p.air == nil {
		return y
	}
	p.air.Write(y)
	return p.air.Read(p.airD)
}
