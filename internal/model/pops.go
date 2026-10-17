package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// golden is the step of the low-discrepancy sequences for pop timing.
const golden = 0.6180339887

// pops is the overrun pop model: the arm and fire logic, the two-level
// scheduler and the pop voice. A pop is a rounded pressure jump with a
// resonant body, injected into an exhaust path, plus a high-frequency
// crack at the outlet.
type pops struct {
	density  float32
	runProb  float32
	minRPM   float32
	fireRPM  float32
	gain     float32
	rate     float32
	interval float32 // Median interval inside a run, in samples.

	armed, window bool
	heat          float32
	heatUp        float32
	heatDown      float32
	prob          float32
	probDecay     float32
	timer         int
	runLeft       int
	uRun, uGap    float32
	target        int

	// Voice.
	busy, busyLen     int
	jumpLeft, jumpLen int
	jumpAmp           float32
	jumpLP            dsp.OnePole
	body              dsp.Biquad
	bodyGain          float32
	jumpMix           float32
	crackLvl          float32
	crackEnv          float32
	crackDec          float32
	crackAtt          float32
	crackRatio        float32
	crackHP           dsp.Biquad
}

// newPops returns the pop model. tier1 lowers the crack and raises the
// body, because the tier 1 expansion chamber takes energy out of the body
// band.
func newPops(s *Spec, rate, chunkSeconds float64, tier1 bool) pops {
	p := s.Pops
	q := pops{
		density:  float32(p.Density),
		runProb:  float32(math.Min(1, 2.5*p.Density)),
		minRPM:   float32(p.MinRPM),
		fireRPM:  float32(1.65 * s.IdleRPM),
		gain:     float32(p.Gain),
		rate:     float32(rate),
		interval: float32(p.IntervalM * 1e-3 * rate),
		heatUp:   dsp.TimeCoef(3, 1/chunkSeconds),
		heatDown: dsp.TimeCoef(15, 1/chunkSeconds),
		// The run probability halves over 2.5 seconds of overrun.
		probDecay: float32(math.Pow(0.5, chunkSeconds/2.5)),
		busyLen:   int(0.3 * rate),
		jumpLen:   max(1, int(p.JumpMS*1e-3*rate)),
		jumpLP:    dsp.OnePole{A: dsp.TimeCoef(0.00025, rate)},
		jumpMix:   0.6,
		bodyGain:  4,
		crackDec:  dsp.DecayCoef(20, p.CrackMS*1e-3, rate),
		crackAtt:  dsp.TimeCoef(0.0001, rate),
		// The crack level is a small share of the body, as measured.
		crackRatio: float32(0.05 * p.Crack),
	}
	if tier1 {
		q.crackRatio *= 0.35
		q.bodyGain *= 1.6
	}
	// The body decays to -20 dB in BodyMS: tau = T / ln 10, Q = pi f tau.
	tau := p.BodyMS * 1e-3 / math.Ln10
	q.body.SetBandPass(p.BodyHz, math.Max(0.5, math.Pi*p.BodyHz*tau), rate)
	q.crackHP.SetHighPass(3800, 0.707, rate)
	return q
}

// control runs once per chunk. heatTarget is the exhaust wall heat that
// the current load drives toward, from 0 to 1.
func (p *pops) control(rpm, throttle float32, overrun, cut bool, heatTarget float32, n *dsp.Noise) {
	if heatTarget > p.heat {
		p.heat += (heatTarget - p.heat) * p.heatUp
	} else {
		p.heat += (heatTarget - p.heat) * p.heatDown
	}
	if p.density <= 0 {
		return
	}
	if throttle > 0.2 && rpm >= p.minRPM {
		p.armed = true
	}
	open := p.armed && overrun && !cut && rpm >= p.minRPM && rpm > p.fireRPM
	switch {
	case open && !p.window:
		p.window = true
		p.prob = 1
		p.runLeft = 0
		p.timer = int((0.04 + 0.06*n.Unipolar()) * p.rate)
	case !open && p.window:
		p.window = false
		p.armed = false
		p.timer = 0
	case open:
		p.prob *= p.probDecay
	}
}

// heatFactor maps the wall heat to a pop gain. A cold exhaust cannot pop.
func (p *pops) heatFactor() float32 {
	return min(1, max(0, (p.heat-0.08)*4))
}

func (p *pops) nextInterval(n *dsp.Noise) int {
	p.uRun += golden
	if p.uRun >= 1 {
		p.uRun--
	}
	f := 1 + 0.42*(2*p.uRun-1) + 0.05*n.Bipolar()
	return max(1, int(p.interval*f))
}

func (p *pops) nextGap(n *dsp.Noise) int {
	p.uGap += golden
	if p.uGap >= 1 {
		p.uGap--
	}
	ms := 300 + 400*p.uGap + 20*n.Bipolar()
	return max(1, int(ms*1e-3*p.rate))
}

// schedule runs when the timer expires inside the overrun window.
func (p *pops) schedule(n *dsp.Noise, paths int) {
	if p.runLeft == 0 {
		if n.Unipolar() >= p.runProb*p.prob*p.heatFactor() {
			p.timer = p.nextGap(n)
			return
		}
		p.runLeft = 1 + int(p.density*10*n.Unipolar())
	}
	p.fire(n, paths, 1)
	p.runLeft--
	if p.runLeft > 0 {
		p.timer = p.nextInterval(n)
	} else {
		p.timer = p.nextGap(n)
	}
}

// fire starts one pop. The per-pop gain spreads over about 9 dB.
func (p *pops) fire(n *dsp.Noise, paths int, scale float32) {
	a := p.gain * scale * p.heatFactor() * (1 - 0.65*n.Unipolar())
	p.jumpAmp = a
	p.jumpLeft = p.jumpLen
	p.crackLvl = a * p.crackRatio
	p.busy = p.busyLen
	p.target++
	if p.target >= paths {
		p.target = 0
	}
}

// misfire is a skipped firing during an ignition cut. Unburnt charge can
// pop in the exhaust.
func (p *pops) misfire(n *dsp.Noise, paths int) {
	if p.density > 0 && p.busy == 0 && n.Unipolar() < p.density {
		p.fire(n, paths, 0.6)
	}
}

// tick runs once per sample. It returns the pressure jump for the target
// path and the crack for the outlet.
func (p *pops) tick(n *dsp.Noise, paths int) (jump, crack float32) {
	if p.timer > 0 {
		p.timer--
		if p.timer == 0 && p.window {
			p.schedule(n, paths)
		}
	}
	if p.busy == 0 {
		return 0, 0
	}
	p.busy--
	var g float32
	if p.jumpLeft > 0 {
		p.jumpLeft--
		g = p.jumpAmp
	}
	j := p.jumpLP.Process(g)
	jump = p.jumpMix*j + p.bodyGain*p.body.Process(j)
	p.crackLvl *= p.crackDec
	p.crackEnv += (p.crackLvl - p.crackEnv) * p.crackAtt
	crack = p.crackHP.Process(n.Bipolar()) * p.crackEnv
	return jump, crack
}
