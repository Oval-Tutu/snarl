package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
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
// rate, a fixed calibration gain, and a look-ahead peak limiter into a
// soft clipper. The engine model sets the level, so no gain here follows
// the signal except the peak limiter.
type output struct {
	dc   dsp.DCBlock
	lp   dsp.Biquad
	gain float32
	lim  limiter
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

func newOutput(gainDB, rate float64) output {
	o := output{
		dc:   dsp.NewDCBlock(10, rate),
		gain: float32(math.Pow(10, gainDB/20)),
		lim:  newLimiter(rate),
	}
	o.lp.SetLowPass(0.45*rate, 0.7071, rate)
	return o
}

// process runs one sample.
func (o *output) process(x float32) float32 {
	x = o.lp.Process(o.dc.Process(x))
	return dsp.SoftClip(o.lim.process(x * o.gain))
}
