package core

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// idle is the idle wander: a slow modulation of the crank speed at idle,
// as the free run at idle of the reference measures it. It draws from its
// own noise source, so that the rest of the engine renders the same
// samples when the wander is zero.
type idle struct {
	noise            dsp.Noise
	wander           dsp.OnePole
	idleRPM, fadeRPM float32
	hunt, phaseStep  float32
	wanderScale      float32
	w, k             float32
	phase            float32

	// speed is the crank speed factor of the control block.
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

// control runs once per control block with the input RPM and the
// smoothed load. The depth falls along a smoothstep over RPM from 1 at
// idle to 0 at the fade RPM, smoothed over 50 ms. It follows the input
// RPM, not the smoothed RPM, so that a start above the fade RPM renders
// exactly as with no wander. The depth also falls linearly with load to 0
// at half load. The modulation is 0.7 times a sine at the hunt rate plus
// 0.5 times a slow random wander with a standard deviation of 1, which
// makes it irregular in depth and rate.
func (d *idle) control(rpm, load float32) {
	d.phase += d.phaseStep
	if d.phase >= 1 {
		d.phase--
	}
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
