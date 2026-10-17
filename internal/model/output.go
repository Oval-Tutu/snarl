package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

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
	// limCeil is the peak limit into the soft clipper. dsp.SoftClip maps
	// it to about 0.975, below full scale.
	limCeil float32 = 1.3
	// limAhead is the look-ahead of the peak limiter in seconds. The gain
	// falls along a straight line over this time, ahead of the peak.
	limAhead = 0.0005
	// limRelease is the release time constant of the peak limiter.
	limRelease = 0.02
)

// output is the output stage: DC block, low pass at 0.45 times the sample
// rate, a peak follower that sets the gain, and a look-ahead peak limiter
// into a soft clipper.
type output struct {
	dc       dsp.DCBlock
	lp       dsp.Biquad
	env      float32
	att, rel float32
	g, gStep float32
	level    float32
	lim      limiter
}

// limiter is a look-ahead peak limiter. A sample above limCeil sets a gain
// ramp that reaches limCeil over its magnitude before the delay line
// releases the sample. The gain holds until the last such sample leaves
// the delay line, then releases.
type limiter struct {
	buf     *dsp.Delay
	n       int
	invN    float32
	g, step float32
	target  float32
	hold    int
	rel     float32
}

func newLimiter(rate float64) limiter {
	n := max(1, int(math.Round(limAhead*rate)))
	return limiter{
		buf:    dsp.NewDelay(n),
		n:      n,
		invN:   1 / float32(n),
		g:      1,
		target: 1,
		rel:    dsp.TimeCoef(limRelease, rate),
	}
}

// process delays x by the look-ahead and returns it with the limiter gain.
func (l *limiter) process(x float32) float32 {
	a := x
	if a < 0 {
		a = -a
	}
	if a > limCeil {
		t := limCeil / a
		switch {
		case l.step < 0:
			l.target = min(l.target, t)
			l.step = min(l.step, (t-l.g)*l.invN)
		case t < l.g:
			l.target = t
			l.step = (t - l.g) * l.invN
		}
		l.hold = l.n
	}
	switch {
	case l.step < 0:
		l.g += l.step
		if l.g <= l.target {
			l.g, l.step = l.target, 0
		}
	case l.hold == 0:
		l.g += (1 - l.g) * l.rel
	}
	if l.hold > 0 {
		l.hold--
	}
	l.buf.Write(x)
	return l.buf.Read(l.n) * l.g
}

func newOutput(level, rate float64) output {
	o := output{
		dc:    dsp.NewDCBlock(10, rate),
		att:   dsp.TimeCoef(0.0005, rate),
		rel:   dsp.TimeCoef(0.4, rate),
		level: float32(level),
		env:   float32(level),
		g:     1,
		lim:   newLimiter(rate),
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
	return dsp.SoftClip(o.lim.process(x * o.g))
}
