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

// The engine-sim leveller (LevelingFilter), with its constants at the
// engine-sim sample rate of 44.1 kHz. levTarget is the output peak that
// the leveller holds above its knee: the engine-sim target of 30000 at a
// volume of 0.5, 0.458 of full scale, lowered by 3.8 dB, so that the
// sweep-wot scenario is near -16 LUFS in place of the -12.2 LUFS of the
// engine-sim references.
const (
	levRate    = 44100
	levRelease = 0.999
	levSmooth  = 0.9
	levMaxGain = 1.9
	levTarget  = 0.296
)

// output is the output stage. The engine passes the leveller gain, a DC
// block and the leveller. The pops join the engine after the leveller, and
// both pass a DC block, a low pass at 0.45 times the sample rate, the
// calibration gain and a look-ahead peak limiter into a soft clipper.
type output struct {
	pre, gain float32
	engDC     dsp.DCBlock
	// noEngDC is true when the formant bank has a high pass, which removes
	// the DC before the leveller, so that engDC does not run.
	noEngDC bool
	lev     leveller
	dc      dsp.DCBlock
	lp      dsp.Biquad
	useLP   bool
	lim     limiter
}

// leveller is the engine-sim peak leveller. The peak follows the input
// magnitude at once and decays by levRelease per sample. The gain moves
// towards levTarget over the peak, at most levMaxGain, by a one-pole
// smoothing. A signal with a peak above levTarget/levMaxGain keeps its
// output peak at levTarget: that is the knee. inv is levTarget over the
// peak, which grows as the peak decays, so that only a new peak needs a
// division. When inv reaches levMaxGain, the peak stops its decay, which
// gives the same gain and keeps the peak away from subnormal values.
type leveller struct {
	peak, inv, g float32
	rel, grow, k float32
}

func newLeveller(rate float64) leveller {
	r := levRate / rate
	rel := math.Pow(levRelease, r)
	return leveller{
		peak: levTarget,
		inv:  1,
		g:    1,
		rel:  float32(rel),
		grow: float32(1 / rel),
		k:    float32(1 - math.Pow(levSmooth, r)),
	}
}

// process levels one sample.
func (l *leveller) process(x float32) float32 {
	a := x
	if a < 0 {
		a = -a
	}
	if l.inv < levMaxGain {
		l.peak *= l.rel
		l.inv *= l.grow
	}
	if a > l.peak {
		l.peak = a
		l.inv = levTarget / a
	}
	l.g += (min(l.inv, levMaxGain) - l.g) * l.k
	return x * l.g
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

// newOutput returns the output stage with the leveller gain and the
// calibration gain in decibels.
func newOutput(levDB, gainDB, rate float64) output {
	o := output{
		pre:   float32(math.Pow(10, levDB/20)),
		gain:  float32(math.Pow(10, gainDB/20)),
		engDC: dsp.NewDCBlock(10, rate),
		lev:   newLeveller(rate),
		dc:    dsp.NewDCBlock(10, rate),
		lim:   newLimiter(rate),
	}
	o.lp.SetLowPass(0.45*rate, 0.7071, rate)
	o.useLP = 0.45*rate < audibleHz
	return o
}

// audibleHz is the highest frequency that the output low pass must keep
// out. At a sample rate of 44.4 kHz and up, 0.45 times the rate is above
// it, so the low pass does not run.
const audibleHz = 20000

// level runs the leveller gain, the DC block and the leveller for one
// engine sample.
func (o *output) level(x float32) float32 {
	x *= o.pre
	if !o.noEngDC {
		x = o.engDC.Process(x)
	}
	return o.lev.process(x)
}

// process runs the rest of the output stage for one sample of the levelled
// engine and the pops.
func (o *output) process(x float32) float32 {
	x = o.dc.Process(x)
	if o.useLP {
		x = o.lp.Process(x)
	}
	return dsp.SoftClip(o.lim.process(x * o.gain))
}
