package model

import "github.com/oval-tutu/snarl/internal/dsp"

// Output gain limits of the levelling stage.
const (
	minGain = 0.05
	maxGain = 3
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

// control sets the gain ramp for the next n samples.
func (o *output) control(n int) {
	if o.env != o.env || o.g != o.g { // NaN: restart the follower.
		o.env, o.g = o.level, 1
	}
	t := o.level / (o.env + 1e-6)
	t = min(maxGain, max(minGain, t))
	o.gStep = (t - o.g) / float32(n)
}

func (o *output) process(x float32) float32 {
	x = o.lp.Process(o.dc.Process(x))
	a := x
	if a < 0 {
		a = -a
	}
	if a > o.env {
		o.env += (a - o.env) * o.att
	} else {
		o.env += (a - o.env) * o.rel
	}
	o.g += o.gStep
	return dsp.SoftClip(x * o.g)
}
