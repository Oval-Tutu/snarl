package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// idle is the low-RPM character: combustion variation in gain and delay,
// cylinder imbalance, a slow idle hunt and a slow rocking. It draws from
// its own noise source, so that the rest of the model renders the same
// samples when every depth is zero.
type idle struct {
	noise                  dsp.Noise
	wander                 dsp.OnePole
	idleRPM, fadeRPM       float32
	variation, imbal, hunt float32
	timing                 float32
	rockDepth, phaseStep   float32
	wanderScale            float32
	w, k                   float32
	phase                  float32

	// Per-chunk outputs: the RPM factor, the pulse gain factor, the extra
	// per-cylinder gain spread, the per-firing gain spread, and the
	// per-firing delay spread as a fraction of the cycle.
	speed, rock, imbalance, spread, delay float32
}

func newIdle(s *Spec, seed uint32, chunkHz float64) idle {
	a := dsp.OnePoleCoef(s.Idle.HuntHz/3, chunkHz)
	return idle{
		k:      dsp.TimeCoef(0.05, chunkHz),
		noise:  dsp.NewNoise(seed + 0x1d1e),
		wander: dsp.OnePole{A: a},
		// The low-passed bipolar noise has a variance of a/(2-a)/3.
		wanderScale: float32(1 / math.Sqrt(float64(a)/(2-float64(a))/3)),
		idleRPM:     float32(s.IdleRPM),
		fadeRPM:     float32(s.Idle.FadeRPM),
		variation:   float32(s.Idle.Variation),
		imbal:       float32(s.Idle.Imbalance),
		hunt:        float32(s.Idle.Hunt),
		timing:      float32(s.Idle.TimingDeg / 720),
		rockDepth:   float32(s.Idle.Rock),
		phaseStep:   float32(s.Idle.HuntHz / chunkHz),
		speed:       1,
		rock:        1,
	}
}

// control runs once per chunk with the input RPM and the smoothed load.
// Each depth falls along a smoothstep over RPM from 1 at idle to 0 at the
// fade RPM, smoothed over 50 ms. It follows the input RPM, not the
// smoothed RPM, so that a start above the fade RPM renders exactly as with
// every depth at zero. The variation, hunt and rocking also fall linearly
// with load to 0 at half load: in engine-sim, the firing gain spread at
// idle at half throttle is that at full throttle (tools/fit --pulse
// --variation), and only the closed throttle adds to it.
func (d *idle) control(rpm, load float32) {
	d.phase += d.phaseStep
	if d.phase >= 1 {
		d.phase--
	}
	// The wander, with a standard deviation of 1, makes the hunt
	// irregular in depth and rate.
	wander := d.wander.Process(d.noise.Bipolar()) * d.wanderScale
	w := min(1, max(0, (d.fadeRPM-rpm)/(d.fadeRPM-d.idleRPM)))
	d.w += (w*w*(3-2*w) - d.w) * d.k
	if d.w < 1e-4 {
		d.w = 0
	}
	w = d.w
	if w <= 0 {
		d.speed, d.rock, d.imbalance, d.spread, d.delay = 1, 1, 0, 0, 0
		return
	}
	d.imbalance = d.imbal * w
	depth := w * max(0, 1-2*load)
	lfo := 0.7*dsp.Sine(d.phase) + 0.5*wander
	d.speed = 1 + d.hunt*depth*lfo
	q := d.phase + 0.25
	if q >= 1 {
		q--
	}
	d.spread = d.variation * depth
	d.delay = d.timing * depth
	// The per-firing spread adds spread^2/3 to the mean square gain. The
	// rocking gain divides it out, so that the variation keeps the level.
	d.rock = (1 + d.rockDepth*depth*(0.7*dsp.Sine(q)-0.5*wander)) /
		float32(math.Sqrt(float64(1+d.spread*d.spread/3)))
}

// firing returns the gain factor and the extra delay, as a fraction of the
// cycle, of one firing.
func (d *idle) firing() (gain, delay float32) {
	return 1 + d.spread*d.noise.Bipolar(), d.delay * d.noise.Unipolar()
}
