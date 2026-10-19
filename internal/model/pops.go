package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// golden is the step of the low-discrepancy sequences for pop timing.
	golden = 0.6180339887
	// bangGain is the gain of an ignition cut bang relative to a pop.
	bangGain = 3
	// maxPopEvents is the number of pop events that can sound at once. A
	// pop that starts when every slot is busy does not sound.
	maxPopEvents = 4
)

// The pop kinds, from the clusters of the muffler crackle pack in
// docs/research/crackles.md.
const (
	kindPop     = iota // An isolated pop with a resonant body.
	kindBoom           // A short deep burst with a bright onset.
	kindCrackle        // A long train of cracks over a deep body.
	kindBurble         // A soft low pop with no crack.
	numKinds
)

// popShape is the form of one pop kind. A pop is a train of sub-events:
// the first starts the pop, and subs more follow at about spacingMS.
type popShape struct {
	bodyRatio float64 // Body frequency over the profile body_hz.
	bodyMS    float64 // Body decay to -20 dB, or 0 for the profile body_ms.
	jump      float64 // Pressure jump length over the profile jump_ms.
	roundMS   float64 // Rounding of the jump edges, or 0 for 0.25 ms.
	gain      float32 // Level over the pop gain.
	crack     float32 // Crack of the first sub-event over the pop crack.
	subCrack  float32 // Crack of a later sub-event over the pop crack.
	subGain   float32 // Jump of a later sub-event over the first.
	subs      int     // Least number of later sub-events.
	subsRange int     // Spread of the number of later sub-events.
	spacingMS float64 // Median spacing of the sub-events.
}

// shapes holds the form of each kind. The body frequencies, decays,
// sub-event counts and spacings are the medians of the pack clusters. The
// gains and crack levels set the band shares of the clusters through the
// exhaust path.
var shapes = [numKinds]popShape{
	kindPop:     {bodyRatio: 1, jump: 1, gain: 1, crack: 1},
	kindBoom:    {bodyRatio: 0.3, bodyMS: 50, jump: 1, gain: 1, crack: 2.5, subCrack: 0.3, subGain: 0.25, subs: 3, subsRange: 12, spacingMS: 12.5},
	kindCrackle: {bodyRatio: 0.3, bodyMS: 70, jump: 0.4, gain: 0.55, crack: 1.5, subCrack: 0.35, subGain: 0.5, subs: 5, subsRange: 27, spacingMS: 12.5},
	kindBurble:  {bodyRatio: 0.8, bodyMS: 34, jump: 3, roundMS: 1.5, gain: 0.125, subGain: 0.6, subs: 0, subsRange: 6, spacingMS: 11},
}

// popEvent is one sounding pop.
type popEvent struct {
	left              int // Samples until the slot is free.
	path              int
	subs, next        int // Sub-events to come and samples to the next.
	spacing           float32
	amp               float32
	jumpLeft, jumpLen int
	jumpAmp           float32
	jumpLP            dsp.OnePole
	body              dsp.Biquad
	crackLvl          float32
	crackEnv          float32
	kind              int
}

// pops is the overrun pop model: the arm and fire logic, the two-level
// scheduler and the pop events. A pop is a rounded pressure jump with a
// resonant body, injected into an exhaust path, plus a high-frequency
// crack at the outlet. Each pop picks a kind with the profile weights.
type pops struct {
	density  float32
	runProb  float32
	minRPM   float32
	fireRPM  float32
	gain     float32
	rate     float32
	interval float32 // Median interval inside a run, in samples.

	// noise is the random source of the pops, apart from the engine, so
	// that the pops do not change the engine sound.
	noise         dsp.Noise
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

	// Events.
	cum        [numKinds]float32 // Cumulative kind weights, from 0 to 1.
	ev         [maxPopEvents]popEvent
	active     int
	count      [numKinds]int // Pops started, per kind.
	jump       [MaxBanks]float32
	busyLen    int
	jumpLen    float64
	jumpA      [numKinds]float32 // Jump rounding coefficient per kind.
	jumpMix    float32
	bodyGain   float32
	bodyHz     float64
	bodyMS     float64
	crackDec   float32
	crackAtt   float32
	crackRatio float32
	crackHP    dsp.Biquad
}

// popCrack scales the crack of the isolated pop. The expansion chamber
// takes energy out of the body band, so the pop crack is lower and the
// body gain is higher.
const popCrack = 0.35

// newPops returns the pop model.
func newPops(s *Spec, rate, chunkSeconds float64) pops {
	p := s.Pops
	q := pops{
		density:  float32(p.Density),
		runProb:  float32(math.Min(1, 2.5*p.Density)),
		minRPM:   float32(p.MinRPM),
		fireRPM:  float32(1.65 * s.IdleRPM),
		gain:     float32(p.Gain),
		rate:     float32(rate),
		noise:    dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x9e37),
		interval: float32(p.IntervalM * 1e-3 * rate),
		heatUp:   dsp.TimeCoef(3, 1/chunkSeconds),
		heatDown: dsp.TimeCoef(15, 1/chunkSeconds),
		// The run probability halves over 2.5 seconds of overrun.
		probDecay: float32(math.Pow(0.5, chunkSeconds/2.5)),
		busyLen:   int(0.3 * rate),
		jumpLen:   p.JumpMS * 1e-3 * rate,
		jumpMix:   0.6,
		bodyGain:  6.4,
		bodyHz:    p.BodyHz,
		bodyMS:    p.BodyMS,
		crackDec:  dsp.DecayCoef(20, p.CrackMS*1e-3, rate),
		crackAtt:  dsp.TimeCoef(0.0001, rate),
		// The crack level is a small share of the body, as measured.
		crackRatio: float32(0.05 * p.Crack),
	}
	for k, sh := range shapes {
		r := sh.roundMS
		if r == 0 {
			r = 0.25
		}
		q.jumpA[k] = dsp.TimeCoef(r*1e-3, rate)
	}
	w := [numKinds]float64{p.WeightPop, p.WeightBoom, p.WeightCrackle, p.WeightBurble}
	var sum, c float64
	for _, v := range w {
		sum += v
	}
	for k, v := range w {
		c += v
		q.cum[k] = float32(c / sum)
		// From the last kind with a weight on, the sum is 1 without
		// rounding, so that no pick falls on a kind with weight 0.
		if c == sum {
			q.cum[k] = 1
		}
	}
	q.crackHP.SetHighPass(3800, 0.707, rate)
	return q
}

// control runs once per chunk. heatTarget is the exhaust wall heat that
// the current load drives toward, from 0 to 1.
func (p *pops) control(rpm, throttle float32, overrun, cut bool, heatTarget float32) {
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
		p.timer = int((0.04 + 0.06*p.noise.Unipolar()) * p.rate)
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

func (p *pops) nextInterval() int {
	p.uRun += golden
	if p.uRun >= 1 {
		p.uRun--
	}
	f := 1 + 0.42*(2*p.uRun-1) + 0.05*p.noise.Bipolar()
	return max(1, int(p.interval*f))
}

func (p *pops) nextGap() int {
	p.uGap += golden
	if p.uGap >= 1 {
		p.uGap--
	}
	ms := 300 + 400*p.uGap + 20*p.noise.Bipolar()
	return max(1, int(ms*1e-3*p.rate))
}

// pick returns a pop kind at random with the profile weights.
func (p *pops) pick() int {
	u := p.noise.Unipolar()
	k := 0
	for k < numKinds-1 && u >= p.cum[k] {
		k++
	}
	return k
}

// schedule runs when the timer expires inside the overrun window.
func (p *pops) schedule(paths int) {
	if p.runLeft == 0 {
		if p.noise.Unipolar() >= p.runProb*p.prob*p.heatFactor() {
			p.timer = p.nextGap()
			return
		}
		p.runLeft = 1 + int(p.density*10*p.noise.Unipolar())
	}
	p.fire(paths, 1, p.pick())
	p.runLeft--
	if p.runLeft > 0 {
		p.timer = p.nextInterval()
	} else {
		p.timer = p.nextGap()
	}
}

// fire starts one pop of a kind in a free slot and returns it, or nil when
// every slot is busy. The per-pop gain spreads over about 9 dB, the body
// frequency over plus or minus 15 per cent and the body decay over plus or
// minus 25 per cent.
func (p *pops) fire(paths int, scale float32, kind int) *popEvent {
	var e *popEvent
	for i := range p.ev {
		if p.ev[i].left == 0 {
			e = &p.ev[i]
			break
		}
	}
	if e == nil {
		return nil
	}
	sh := &shapes[kind]
	a := p.gain * scale * p.heatFactor() * (1 - 0.65*p.noise.Unipolar()) * sh.gain
	ms := sh.bodyMS
	if ms == 0 {
		ms = p.bodyMS
	}
	ms *= 0.75 + 0.5*float64(p.noise.Unipolar())
	hz := p.bodyHz * sh.bodyRatio * (0.85 + 0.3*float64(p.noise.Unipolar()))
	// The body decays to -20 dB in ms: tau = T / ln 10, Q = pi f tau.
	tau := ms * 1e-3 / math.Ln10
	e.body.SetBandPass(hz, math.Max(0.5, math.Pi*hz*tau), float64(p.rate))
	e.jumpLP = dsp.OnePole{A: p.jumpA[kind]}
	e.jumpLen = max(1, int(p.jumpLen*sh.jump))
	e.jumpLeft = e.jumpLen
	e.jumpAmp = a
	e.amp = a
	e.crackLvl = a * p.crackRatio * sh.crack
	if kind == kindPop {
		e.crackLvl *= popCrack
	}
	e.kind = kind
	e.subs = sh.subs + int(float32(sh.subsRange+1)*p.noise.Unipolar())
	e.spacing = float32(sh.spacingMS*1e-3) * p.rate
	e.next = p.subSpacing(e)
	// The slot holds the train and six body decays, down by 120 dB.
	e.left = max(p.busyLen, int(float32(e.subs)*1.6*e.spacing)+int(6*ms*1e-3*float64(p.rate)))
	e.path = p.target
	p.target++
	if p.target >= paths {
		p.target = 0
	}
	p.active++
	p.count[kind]++
	return e
}

// subSpacing returns the samples to the next sub-event, from 0.65 to 1.55
// times the median spacing.
func (p *pops) subSpacing(e *popEvent) int {
	return max(1, int(e.spacing*(0.65+0.9*p.noise.Unipolar())))
}

// bang is the bang of unburnt fuel in an ignition cut: a pop with a
// shorter pressure jump, so that its attack is sharper.
func (p *pops) bang(paths int) {
	if e := p.fire(paths, bangGain, kindPop); e != nil {
		e.jumpLen = max(1, e.jumpLen/3)
		e.jumpLeft = e.jumpLen
	}
}

// tick runs once per sample. It sets the pressure jump of each path in
// p.jump and returns the crack for the outlet.
func (p *pops) tick(paths int) (crack float32) {
	if p.timer > 0 {
		p.timer--
		if p.timer == 0 && p.window {
			p.schedule(paths)
		}
	}
	if p.active == 0 {
		return 0
	}
	p.jump = [MaxBanks]float32{}
	var env float32
	for i := range p.ev {
		e := &p.ev[i]
		if e.left == 0 {
			continue
		}
		e.left--
		if e.left == 0 {
			p.active--
		}
		if e.subs > 0 {
			e.next--
			if e.next == 0 {
				sh := &shapes[e.kind]
				u := 0.3 + 0.7*p.noise.Unipolar()
				e.jumpLeft = e.jumpLen
				e.jumpAmp = e.amp * sh.subGain * u
				e.crackLvl += e.amp * p.crackRatio * sh.subCrack * u
				e.subs--
				e.next = p.subSpacing(e)
			}
		}
		var g float32
		if e.jumpLeft > 0 {
			e.jumpLeft--
			g = e.jumpAmp
		}
		j := e.jumpLP.Process(g)
		p.jump[e.path] += p.jumpMix*j + p.bodyGain*e.body.Process(j)
		e.crackLvl *= p.crackDec
		e.crackEnv += (e.crackLvl - e.crackEnv) * p.crackAtt
		env += e.crackEnv
	}
	crack = p.crackHP.Process(p.noise.Bipolar()) * env
	if p.active == 0 {
		p.jump = [MaxBanks]float32{}
	}
	return crack
}
