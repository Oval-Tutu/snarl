package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// idle is the idle hunt: a slow RPM modulation at idle. It draws from its
// own noise source, so that the rest of the model renders the same samples
// when the hunt is zero.
type idle struct {
	noise            dsp.Noise
	wander           dsp.OnePole
	idleRPM, fadeRPM float32
	hunt, phaseStep  float32
	wanderScale      float32
	w, k             float32
	phase            float32

	// speed is the RPM factor of the chunk.
	speed float32
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
		hunt:        float32(s.Idle.Hunt),
		phaseStep:   float32(s.Idle.HuntHz / chunkHz),
		speed:       1,
	}
}

// control runs once per chunk with the input RPM and the smoothed load.
// The depth falls along a smoothstep over RPM from 1 at idle to 0 at the
// fade RPM, smoothed over 50 ms. It follows the input RPM, not the
// smoothed RPM, so that a start above the fade RPM renders exactly as with
// no hunt. The depth also falls linearly with load to 0 at half load.
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
	if d.w <= 0 {
		d.speed = 1
		return
	}
	depth := d.w * max(0, 1-2*load)
	d.speed = 1 + d.hunt*depth*(0.7*dsp.Sine(d.phase)+0.5*wander)
}
