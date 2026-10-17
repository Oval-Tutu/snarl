package model

import "github.com/oval-tutu/snarl/internal/dsp"

// Output gain limits of the levelling stage. On overrun, the pulses are
// much weaker than at full load, so the limit rises to overrunMaxGain to
// hold the overrun level that the profile sets.
const (
	minGain        = 0.05
	maxGain        = 3
	overrunMaxGain = 12
	// popHeadroom is how far a pop can rise above the level before it
	// lowers the gain.
	popHeadroom float32 = 1.6
)

// output is the output stage: DC block, low pass at 0.45 times the sample
// rate, and a peak follower that sets the gain into a soft clipper.
type output struct {
	dc       dsp.DCBlock
	lp       dsp.Biquad
	env      float32
	att, rel float32
	g, gStep float32
	level    float32
}

func newOutput(level, rate float64) output {
	o := output{
		dc:    dsp.NewDCBlock(10, rate),
		att:   dsp.TimeCoef(0.0005, rate),
		rel:   dsp.TimeCoef(0.4, rate),
		level: float32(level),
		env:   float32(level),
		g:     1,
	}
	o.lp.SetLowPass(0.45*rate, 0.7071, rate)
	return o
}

// control sets the gain ramp for the next n samples. scale is the target
// level relative to the profile level, and ceil is the gain limit.
func (o *output) control(n int, scale, ceil float32) {
	if o.env != o.env || o.g != o.g { // NaN: restart the follower.
		o.env, o.g = o.level, 1
	}
	t := o.level * scale / (o.env + 1e-6)
	t = min(ceil, max(minGain, t))
	o.gStep = (t - o.g) / float32(n)
}

// process runs one sample. While hold is true, a pop sounds, and the
// follower rises only for the part of the pop above popHeadroom times the
// level. So the pop sounds on top of the engine and lowers the gain only
// when it is very loud.
func (o *output) process(x float32, hold bool) float32 {
	x = o.lp.Process(o.dc.Process(x))
	a := x
	if a < 0 {
		a = -a
	}
	d := a
	if hold {
		d *= 1 / popHeadroom
	}
	if d > o.env {
		o.env += (d - o.env) * o.att
	} else if a < o.env {
		o.env += (a - o.env) * o.rel
	}
	o.g += o.gStep
	return dsp.SoftClip(x * o.g)
}
