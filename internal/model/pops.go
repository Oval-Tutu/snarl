package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// golden is the step of the low-discrepancy sequence for the pop
	// intervals inside a lift-off run.
	golden = 0.6180339887
	// bangGain is the gain of an ignition cut bang relative to a pop.
	bangGain = 3
	// maxPopEvents is the number of pop events that can sound at once. An
	// own pop or a bang that starts when every slot is busy does not
	// sound. A requested pop takes the slot of the oldest own pop, or of
	// the oldest pop when no own pop sounds.
	maxPopEvents = 4
	// densityScale maps the profile density to the multiplier of the own
	// pop chances.
	densityScale = 5
	// histSlots is the number of slots of the throttle history, and
	// histSeconds the length of one slot. The history holds the highest
	// throttle of the last 150 to 160 ms.
	histSlots   = 16
	histSeconds = 0.01
	// ownWindow is the number of own pops that can start in ownSeconds.
	ownWindow  = 6
	ownSeconds = 10
	// shortCrackle is the most sub-events of a requested crackle.
	shortCrackle = 10
	// gapSteps is the number of steps per unit of the normal value in
	// the click gap table, and gapRange the highest normal value in it.
	gapSteps = 8
	gapRange = 3
)

// The pop timing in seconds, chances and gates. They are starting points
// for tuning by ear.
const (
	guardSec     = 0.05 // No pop starts within this time of the last.
	gapSec       = 0.7  // Least time between own pops of two triggers.
	liftFall     = 0.5  // Throttle fall in the history that is a lift-off.
	liftRedline  = 0.6  // Least RPM of a lift-off, over the redline.
	liftChance   = 0.8  // Chance of the first pop of a lift-off run.
	liftDecay    = 0.6  // Chance of each next pop over the one before.
	liftMaxPops  = 3    // Most pops in a lift-off run.
	liftDelayMin = 0.15 // Delay from the lift to the first pop.
	liftDelayMax = 0.4
	runGapMin    = 0.11 // Interval inside a run.
	runGapMax    = 0.264
	runEndMin    = 1.5 // End of the run after the lift.
	runEndMax    = 2.5
	liftRestSec  = 2   // Rest after the end of a run.
	windIdle     = 1.3 // Lowest RPM of a wind-down band, over idle.
	windBandRPM  = 1000
	windHyst     = 150  // RPM rise that arms a band again.
	windChance   = 0.55 // Chance of a pop or a burble in each band.
)

// overDB holds the level of each pop kind in decibels over the engine: the
// RMS level of the pop over the span from its start that holds 90 per cent
// of its energy, against the RMS level of the engine with no pop over the
// same span. ownNorm brings the median level of an own pop, 0.675, to 1,
// so that the median own pop has its target.
var overDB = [numKinds]float64{kindPop: 4.8, kindBoom: 4.5, kindCrackle: 1.5, kindBurble: -4.3}

const ownNorm = 1 / 0.675

// The calibration of the kind gains renders calDraws pops of up to calSec
// of each kind on each path. calTrimDB raises the gains, because the
// median of the random pops in an engine is about 0.5 dB below the mean
// level of the calibration pops.
const (
	calSec    = 0.8
	calDraws  = 7
	calTrimDB = 0.5
)

// The crackle clicks. A click is a noise burst through a band pass at
// clickHz that starts at once and decays to -20 dB in clickSec. The gaps
// between the clicks are log-normal around gapMS with the spread
// gapSigma, and the normal value of each gap follows the one before with
// the correlation gapRho. The values put the clicks, measured as
// TestCrackleClicks measures them, on the clicks of the crackle trains in
// the muffler crackle pack (docs/research/crackles.md section 11).
const (
	clickSec = 0.0095
	clickHz  = 1500
	clickQ   = 1.2
	gapMS    = 6.5
	gapSigma = 0.95
	gapRho   = 0.1
)

// The pop kinds, from the clusters of the muffler crackle pack in
// docs/research/crackles.md. The snarl PopKind values are these plus 1.
const (
	kindPop     = iota // An isolated pop with a resonant body.
	kindBoom           // A short deep burst with a bright onset.
	kindCrackle        // A long train of cracks over a deep body.
	kindBurble         // A soft low pop with no crack.
	numKinds
)

// The pop sources, with the values of the snarl PopSource type.
const (
	srcRequest = iota + 1 // A pop that the game requests.
	srcOwn                // A pop of a lift-off or a wind-down.
	srcLimiter            // An ignition cut bang.
)

// The own pop triggers.
const (
	trigLift = iota + 1
	trigWind
)

// The pop characters, in the order of the snarl PopCharacter values from 1.
const (
	charStock = iota
	charSport
	charRace
	numCharacters
)

// characterNames are the profile names of the characters.
var characterNames = [numCharacters]string{"stock", "sport", "race"}

// character scales the own pops and, by less, the requested pops. first
// and later are the kinds that the first pop and the later pops of a
// lift-off run can pick, with the profile weights.
type character struct {
	density           float64
	gainDB, requestDB float64
	first, later      [numKinds]bool
}

var characters = [numCharacters]character{
	charStock: {
		density: 0.5, gainDB: -6, requestDB: -2,
		first: [numKinds]bool{true, true, true, true}, later: [numKinds]bool{kindPop: true, kindBurble: true},
	},
	charSport: {
		density: 1, gainDB: 0, requestDB: 0,
		first: [numKinds]bool{kindBoom: true, kindCrackle: true}, later: [numKinds]bool{kindPop: true, kindBurble: true},
	},
	charRace: {
		density: 1.8, gainDB: 3, requestDB: 1,
		first: [numKinds]bool{kindBoom: true, kindCrackle: true}, later: [numKinds]bool{true, true, true, true},
	},
}

// characterIndex returns the character with the profile name, and false
// for an unknown name.
func characterIndex(name string) (int, bool) {
	for i, n := range characterNames {
		if n == name {
			return i, true
		}
	}
	return 0, false
}

// popShape is the form of one pop kind. A pop is a train of sub-events:
// the first starts the pop, and subs more follow at about spacingMS.
type popShape struct {
	bodyRatio float64 // Body frequency over the profile body_hz.
	bodyMS    float64 // Body decay to -20 dB, or 0 for the profile body_ms.
	jump      float64 // Pressure jump length over the profile jump_ms.
	roundMS   float64 // Rounding of the jump edges, or 0 for 0.25 ms.
	crack     float32 // Crack of the first sub-event over the pop crack.
	subCrack  float32 // Crack of a later sub-event over the pop crack.
	subGain   float32 // Jump of a later sub-event over the first.
	subs      int     // Least number of later sub-events.
	subsRange int     // Spread of the number of later sub-events.
	spacingMS float64 // Median spacing of the sub-events.
	click     bool    // Sub-events are short clicks at irregular gaps.
}

// shapes holds the form of each kind. The body frequencies, decays,
// sub-event counts and spacings are the medians of the pack clusters. The
// crack levels set the band shares of the clusters through the exhaust
// path. The kind gains of pops.calibrate set the levels.
var shapes = [numKinds]popShape{
	kindPop:     {bodyRatio: 1, jump: 1, crack: 1},
	kindBoom:    {bodyRatio: 0.3, bodyMS: 50, jump: 1, crack: 2.5, subCrack: 0.3, subGain: 0.25, subs: 3, subsRange: 12, spacingMS: 12.5},
	kindCrackle: {bodyRatio: 0.3, bodyMS: 70, jump: 0.4, crack: 1.5, subCrack: 1.3, subGain: 0.5, subs: 5, subsRange: 27, spacingMS: gapMS, click: true},
	kindBurble:  {bodyRatio: 0.8, bodyMS: 34, jump: 3, roundMS: 1.5, subGain: 0.6, subs: 0, subsRange: 6, spacingMS: 11},
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
	clickLvl          float32
	gapZ              float32 // Normal value of the last click gap.
	kind, src         int
	start             int64 // Start time in samples.
}

// popInput is the engine state of one control block for the pops.
type popInput struct {
	// rpm is the smoothed RPM, and throttle the input throttle.
	rpm, throttle float32
	// overrun is true on overrun with the engine running, and cut is true
	// in a limiter cut or climb.
	overrun, cut bool
	// off stops the own pops.
	off bool
	// heat is the exhaust wall heat that the load drives toward, from 0
	// to 1.
	heat float32
	// character is 0 for the profile character, or the character plus 1.
	character int
	// level is the RMS level of the engine with no pop. Every pop scales
	// by it.
	level    float32
	n, paths int
}

// pops is the pop model: the requested pops, the own pops of a lift-off
// or a wind-down, the limits of the own pops and the pop events. All
// triggers run once per control block. A pop is a rounded pressure jump
// with a resonant body, injected into an exhaust path, plus a
// high-frequency crack at the outlet.
type pops struct {
	density    float32 // Multiplier of the own pop chances.
	bangChance float32
	gain       float32
	rate       float32
	character  int
	liftRPM    float32
	level      float32

	// Wind-down bands below liftRPM. windInv is the number of bands per
	// RPM, windBands the number of bands, and windIn the number of band
	// tops at or above the RPM that rolled their chance.
	windInv   float32
	windBands int
	windIn    int

	// noise is the random source of the pops, apart from the engine, so
	// that the pops do not change the engine sound.
	noise    dsp.Noise
	heat     float32
	heatUp   float32
	heatDown float32

	// Times in samples. now is the start of the control block, and last
	// the start of the last pop. The other times are the lengths of the
	// timing constants.
	now, last          int64
	guard, gap, window int64

	// Requests. reqKind is the kind of the last requested pop, or -1.
	reqPending  int
	reqStrength float32
	reqKind     int

	// Throttle history: the highest throttle of each slot, and histTop
	// the highest of the slots before the current one.
	hist           [histSlots]float32
	histTop        float32
	histI, histAcc int
	histLen        int

	// Lift-off run. runLeft is the pops to come, at runAt or later, up to
	// runEnd. No lift-off triggers before liftRest.
	runLeft         int
	runFirst        bool
	runAt, runEnd   int64
	liftRest        int64
	uRun            float32
	ownTimes        [ownWindow]int64 // Start times of the last own pops.
	ownI            int              // Index of the oldest in ownTimes.
	lastOwn         int64
	lastTrig        int
	target          int                                 // Path of the next pop.
	cum             [numCharacters][2][numKinds]float32 // Run kind weights: first pop, later pops.
	gainOwn, gainRq [numCharacters]float32
	// sounded is true when a pop sounded since the last control block.
	sounded bool
	// stratum is the draw of the calibration from 1 to calDraws, or 0
	// outside it. drawN counts the draws of the form of a pop, and subU
	// is the last draw for a sub-event.
	stratum, drawN int
	subU           float32

	// Report: pops started, and the kind, source and strength of the last.
	total        uint32
	lastKind     int
	lastSrc      int
	lastStrength float32

	// Events.
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
	clickDec   float32
	clickBP    dsp.Biquad
	gapTab     [2*gapSteps*gapRange + 1]float32 // Gap over gapMS per normal value step.
	gapInnov   float32                          // Gain of the new draw, sqrt(1-gapRho^2).
	gapMean    float32                          // Mean gap over the median, exp(gapSigma^2/2).
	// kindGain is the gain of each kind on each path that puts a pop of
	// unit scale on an engine of unit RMS level at its overDB target, for
	// an own pop or a bang and for a requested pop.
	kindGain [MaxBanks][2][numKinds]float32
}

// popCrack scales the crack of the isolated pop. The expansion chamber
// takes energy out of the body band, so the pop crack is lower and the
// body gain is higher.
const popCrack = 0.35

// newPops returns the pop model.
func newPops(s *Spec, rate, chunkSeconds float64) pops {
	p := s.Pops
	ch, _ := characterIndex(p.Character)
	q := pops{
		density:    float32(densityScale * p.Density),
		bangChance: float32(2 * p.Density),
		gain:       float32(p.Gain),
		rate:       float32(rate),
		character:  ch,
		liftRPM:    float32(liftRedline * s.RedlineRPM),
		noise:      dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x9e37),
		heatUp:     dsp.TimeCoef(3, 1/chunkSeconds),
		heatDown:   dsp.TimeCoef(15, 1/chunkSeconds),
		guard:      int64(guardSec * rate),
		gap:        int64(gapSec * rate),
		window:     int64(ownSeconds * rate),
		reqKind:    -1,
		histLen:    int(histSeconds * rate),
		busyLen:    int(0.3 * rate),
		jumpLen:    p.JumpMS * 1e-3 * rate,
		jumpMix:    0.6,
		bodyGain:   6.4,
		bodyHz:     p.BodyHz,
		bodyMS:     p.BodyMS,
		crackDec:   dsp.DecayCoef(20, p.CrackMS*1e-3, rate),
		crackAtt:   dsp.TimeCoef(0.0001, rate),
		// The crack level is a small share of the body, as measured.
		crackRatio: float32(0.05 * p.Crack),
		clickDec:   dsp.DecayCoef(20, clickSec, rate),
		gapInnov:   float32(math.Sqrt(1 - gapRho*gapRho)),
		gapMean:    float32(math.Exp(gapSigma * gapSigma / 2)),
	}
	if span := float64(q.liftRPM) - windIdle*s.IdleRPM; span > 0 {
		q.windBands = max(1, int(math.Round(span/windBandRPM)))
		q.windInv = float32(float64(q.windBands) / span)
	}
	// The engine starts below the bands, so that the RPM rising from 0
	// rolls no band.
	q.windIn = q.windBands
	for i := range q.gapTab {
		z := float64(i)/gapSteps - gapRange
		q.gapTab[i] = float32(math.Exp(gapSigma * z))
	}
	q.last = -q.guard
	for i := range q.ownTimes {
		q.ownTimes[i] = -q.window
	}
	q.lastOwn = -q.gap
	for k, sh := range shapes {
		r := sh.roundMS
		if r == 0 {
			r = 0.25
		}
		q.jumpA[k] = dsp.TimeCoef(r*1e-3, rate)
	}
	w := [numKinds]float64{p.WeightPop, p.WeightBoom, p.WeightCrackle, p.WeightBurble}
	for c, cs := range characters {
		q.cum[c][0] = cumulative(w, cs.first)
		q.cum[c][1] = cumulative(w, cs.later)
		q.gainOwn[c] = float32(math.Pow(10, cs.gainDB/20))
		q.gainRq[c] = float32(math.Pow(10, cs.requestDB/20))
	}
	q.crackHP.SetHighPass(3800, 0.707, rate)
	q.clickBP.SetBandPass(clickHz, clickQ, rate)
	for i := range q.kindGain {
		for r := range q.kindGain[i] {
			for k := range q.kindGain[i][r] {
				q.kindGain[i][r][k] = 1
			}
		}
	}
	return q
}

// calibrate sets the kind gains for the exhaust paths of the spec. It
// renders calDraws pops of each kind at unit gain into each path, from a
// copy of the pop model, and measures the RMS level over the span that
// holds 90 per cent of the energy, through the DC block and the low pass
// of the output stage, with no output gain or limiter. The gain is set
// from the mean level in decibels. The body frequency, body decay and
// sub-event count of the pops cover their spreads in even steps, and the
// sub-events follow a low-discrepancy sequence. The render runs at half
// the sample rate, which keeps the level of the body and the crack and
// halves the cost. It runs once in New and allocates.
func (p *pops) calibrate(s *Spec, rate float64) {
	rate /= 2
	buf := make([]float32, int(calSec*rate))
	step := int(0.005 * rate)
	c := newPops(s, rate, chunk/rate)
	c.noise = dsp.NewNoise(0xca1)
	for i, ps := range s.Paths {
		for slot := range 2 * numKinds {
			kind, src := slot%numKinds, srcOwn
			if slot >= numKinds {
				src = srcRequest
			}
			var lg float64
			for d := range calDraws {
				c.target, c.stratum, c.drawN = i, d+1, 0
				c.subU = float32(d) / calDraws
				e := c.fire(len(s.Paths), kind, src, 1, 1, 1)
				q := newPath(ps, s.Exhaust, rate)
				o := newOutput(0, rate)
				// The render stops when the train has ended and a step is
				// 40 dB below the loudest step.
				var top float64
				n := 0
				for n+step <= len(buf) {
					var en float64
					for j := n; j < n+step; j++ {
						crack := c.tick()
						v := o.lp.Process(o.dc.Process(q.process(0, c.jump[i], false) + crack))
						buf[j] = v
						en += float64(v) * float64(v)
					}
					n += step
					top = max(top, en)
					if e.subs == 0 && en < 1e-4*top {
						break
					}
				}
				e.left, c.active, c.stratum = 0, 0, 0
				lg += math.Log10(math.Max(spanRMS(buf[:n], int(0.01*rate)), 1e-9))
			}
			p.kindGain[i][slot/numKinds][kind] = float32(math.Pow(10, (overDB[kind]+calTrimDB)/20-lg/calDraws))
		}
	}
}

// spanRMS returns the RMS level of x from its start to the sample at which
// the energy reaches 90 per cent of the total, and over at least minN
// samples.
func spanRMS(x []float32, minN int) float64 {
	var tot float64
	for _, v := range x {
		tot += float64(v) * float64(v)
	}
	var e float64
	for i, v := range x {
		e += float64(v) * float64(v)
		if e >= 0.9*tot {
			n := min(len(x), max(i+1, minN))
			var w float64
			for _, u := range x[:n] {
				w += float64(u) * float64(u)
			}
			return math.Sqrt(w / float64(n))
		}
	}
	return 0
}

// cumulative returns the cumulative weights of the kinds in use, from 0 to
// 1. When no kind in use has a weight, every kind is in use.
func cumulative(w [numKinds]float64, use [numKinds]bool) [numKinds]float32 {
	var sum float64
	for k, v := range w {
		if use[k] {
			sum += v
		}
	}
	if sum == 0 {
		return cumulative(w, [numKinds]bool{true, true, true, true})
	}
	var cum [numKinds]float32
	var c float64
	for k, v := range w {
		if use[k] {
			c += v
		}
		cum[k] = float32(c / sum)
		// From the last kind with a weight on, the sum is 1 without
		// rounding, so that no pick falls on a kind with weight 0.
		if c == sum {
			cum[k] = 1
		}
	}
	return cum
}

// sec returns seconds in samples.
func (p *pops) sec(s float64) int64 {
	return int64(s * float64(p.rate))
}

// uniform returns a random time in samples from lo to hi seconds.
func (p *pops) uniform(lo, hi float64) int64 {
	return p.sec(lo + (hi-lo)*float64(p.noise.Unipolar()))
}

// request adds n requested pops with the strength of the latest, from 0
// to 1. At most four wait, so that a game that sends requests faster than
// the guard does not build a long queue.
func (p *pops) request(n int, strength float32) {
	p.reqPending = min(p.reqPending+n, maxPopEvents)
	if !(strength >= 0) { // Negative or NaN.
		strength = 0
	}
	p.reqStrength = min(strength, 1)
}

// control runs once per control block.
func (p *pops) control(in *popInput) {
	if in.heat > p.heat {
		p.heat += (in.heat - p.heat) * p.heatUp
	} else {
		p.heat += (in.heat - p.heat) * p.heatDown
	}
	p.level = in.level
	p.trigger(in, p.now)
	p.now += int64(in.n)
}

// trigger runs the requested pops and the own triggers for the block that
// starts at now.
func (p *pops) trigger(in *popInput, now int64) {
	fall := p.history(in.throttle, in.n)
	ch := p.character
	if in.character > 0 && in.character <= numCharacters {
		ch = in.character - 1
	}
	if p.reqPending > 0 && now-p.last >= p.guard {
		p.requested(in, ch)
	}
	over := in.overrun && !in.cut
	if !over {
		p.runLeft = 0
	}
	band := p.windBand(in.rpm, over)
	if in.off || p.density <= 0 {
		p.runLeft = 0
		return
	}
	m := p.density * float32(characters[ch].density) * p.heatFactor()
	if over && m > 0 && fall > liftFall && in.rpm > p.liftRPM && now >= p.liftRest {
		p.lift(m, now)
	}
	if p.runLeft > 0 && now >= p.runAt {
		switch {
		case now >= p.runEnd:
			p.runLeft = 0
		case p.ownAllowed(now, trigLift):
			pos := 1
			if p.runFirst {
				pos = 0
			}
			p.own(in, ch, pick(p.noise.Unipolar(), &p.cum[ch][pos]), trigLift, now)
			p.runLeft--
			p.runFirst = false
			p.runAt = now + p.nextInterval()
		}
	}
	if band && p.noise.Unipolar() < windChance*m && p.ownAllowed(now, trigWind) {
		// The share of pops falls from 0.8 in the top band to 0.2 in the
		// lowest.
		k := kindBurble
		if p.noise.Unipolar() < 0.8-0.6*float32(p.windIn-1)/float32(max(1, p.windBands-1)) {
			k = kindPop
		}
		p.own(in, ch, k, trigWind, now)
	}
}

// windBand tracks the wind-down bands. The bands split the RPM from
// liftRPM down to windIdle times idle. It reports whether the RPM on
// overrun fell into a band that has not rolled its chance. A band rolls
// again after the RPM rises windHyst above its top, or after the overrun
// ends.
func (p *pops) windBand(rpm float32, over bool) bool {
	if p.windBands == 0 {
		return false
	}
	// in returns the number of band tops at or above r.
	in := func(r float32) int {
		if r > p.liftRPM {
			return 0
		}
		return min(p.windBands, int((p.liftRPM-r)*p.windInv)+1)
	}
	n := in(rpm)
	switch {
	case !over:
		p.windIn = n
	case n > p.windIn:
		p.windIn = n
		return true
	default:
		p.windIn = min(p.windIn, in(rpm-windHyst))
	}
	return false
}

// history adds the throttle of a block of n samples to the throttle
// history, and returns the fall from the highest throttle in it.
func (p *pops) history(thr float32, n int) float32 {
	p.histAcc += n
	if p.histAcc >= p.histLen {
		p.histAcc -= p.histLen
		p.histI = (p.histI + 1) % histSlots
		p.hist[p.histI] = 0
		p.histTop = 0
		for _, v := range p.hist {
			p.histTop = max(p.histTop, v)
		}
	}
	p.hist[p.histI] = max(p.hist[p.histI], thr)
	return max(p.histTop, p.hist[p.histI]) - thr
}

// lift starts a lift-off run with the chance multiplier m. The run length
// is drawn now: the first pop has the lift-off chance, and each next pop
// liftDecay times the chance of the pop before. A run of no pops still
// starts the rest.
func (p *pops) lift(m float32, now int64) {
	p.runEnd = now + p.uniform(runEndMin, runEndMax)
	p.liftRest = p.runEnd + p.sec(liftRestSec)
	c := liftChance * m
	p.runLeft = 0
	for p.runLeft < liftMaxPops && p.noise.Unipolar() < c {
		p.runLeft++
		c *= liftDecay
	}
	p.runFirst = true
	p.runAt = now + p.uniform(liftDelayMin, liftDelayMax)
}

// ownAllowed reports whether an own pop of the trigger can start now: the
// guard has passed, fewer than ownWindow own pops started in the last
// ownSeconds, and gapSec has passed since an own pop of another trigger.
func (p *pops) ownAllowed(now int64, trig int) bool {
	return now-p.last >= p.guard && now-p.ownTimes[p.ownI] >= p.window &&
		(trig == p.lastTrig || now-p.lastOwn >= p.gap)
}

// own starts an own pop of a kind for the trigger. Its gain spreads over
// about 9 dB and falls on a cold exhaust.
func (p *pops) own(in *popInput, ch, kind, trig int, now int64) {
	level := 1 - 0.65*p.noise.Unipolar()
	if p.fire(in.paths, kind, srcOwn, p.gain*p.gainOwn[ch]*p.heatFactor()*p.level*ownNorm, level, level) == nil {
		return
	}
	p.ownTimes[p.ownI] = now
	p.ownI = (p.ownI + 1) % ownWindow
	p.lastOwn, p.lastTrig = now, trig
}

// requested starts one requested pop. The strength picks two kinds: below
// 1/3 a pop or a burble, below 2/3 a pop or a boom, and above that a boom
// or a short crackle. The pick is random, but never the kind of the last
// requested pop. The gain spreads over plus or minus 3 dB around the level
// target of the kind over the engine. The pop does not depend on the wall
// heat.
func (p *pops) requested(in *popInput, ch int) {
	s := p.reqStrength
	a, b := kindBoom, kindCrackle
	switch {
	case s < 1.0/3:
		a, b = kindPop, kindBurble
	case s < 2.0/3:
		a, b = kindPop, kindBoom
	}
	k := a
	if p.noise.Unipolar() < 0.5 {
		k = b
	}
	if k == p.reqKind {
		k = a + b - k
	}
	p.reqKind = k
	scale := p.gain * p.gainRq[ch] * p.level
	level := float32(math.Pow(10, 0.15*float64(p.noise.Bipolar())))
	p.fire(in.paths, k, srcRequest, scale, level, s)
	p.reqPending--
}

// b2i returns 1 for true and 0 for false.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// draw returns a random value from 0 to 1 for the form of a pop at its
// start. In the calibration, the draws of the pops cover the range in
// calDraws even steps, each form value in another order.
func (p *pops) draw() float32 {
	if p.stratum == 0 {
		return p.noise.Unipolar()
	}
	p.drawN++
	return (float32(p.stratum*p.drawN%calDraws) + 0.5) / calDraws
}

// subDraw returns a random value from 0 to 1 for a sub-event. In the
// calibration, it is a low-discrepancy sequence from the draw.
func (p *pops) subDraw() float32 {
	if p.stratum == 0 {
		return p.noise.Unipolar()
	}
	p.subU += golden
	if p.subU >= 1 {
		p.subU--
	}
	return p.subU
}

// heatFactor maps the wall heat to a pop gain. A cold exhaust cannot pop.
func (p *pops) heatFactor() float32 {
	return min(1, max(0, (p.heat-0.08)*4))
}

// nextInterval returns the samples to the next pop of a run, from
// runGapMin to runGapMax.
func (p *pops) nextInterval() int64 {
	p.uRun += golden
	if p.uRun >= 1 {
		p.uRun--
	}
	u := min(1, max(0, p.uRun+0.05*p.noise.Bipolar()))
	return p.sec(runGapMin + (runGapMax-runGapMin)*float64(u))
}

// pick returns the kind of the random value u from 0 to 1 with the
// cumulative weights cum.
func pick(u float32, cum *[numKinds]float32) int {
	k := 0
	for k < numKinds-1 && u >= cum[k] {
		k++
	}
	return k
}

// slot returns the slot for a pop of the source. It is a free slot, or
// for a requested pop the slot of the oldest own pop, or of the oldest pop
// when no own pop sounds. Otherwise it is nil.
func (p *pops) slot(src int) *popEvent {
	var oldOwn, old *popEvent
	for i := range p.ev {
		e := &p.ev[i]
		if e.left == 0 {
			return e
		}
		if e.src == srcOwn && (oldOwn == nil || e.start < oldOwn.start) {
			oldOwn = e
		}
		if old == nil || e.start < old.start {
			old = e
		}
	}
	if src != srcRequest {
		return nil
	}
	if oldOwn != nil {
		old = oldOwn
	}
	old.left = 0
	p.active--
	return old
}

// fire starts one pop of a kind from the source and returns it, or nil
// when no slot is free. The gain is scale times level times the kind gain.
// The scale of an own pop, a requested pop and a bang holds the engine
// level, so that each kind keeps its level over the engine.
// strength is the strength in the pop report. The body frequency spreads
// over plus or minus 15 per cent, or 8 per cent for a requested pop, and
// the body decay over plus or minus 25 per cent.
func (p *pops) fire(paths, kind, src int, scale, level, strength float32) *popEvent {
	e := p.slot(src)
	if e == nil {
		return nil
	}
	sh := &shapes[kind]
	e.subs = sh.subs + int(float32(sh.subsRange+1)*p.draw())
	e.path = p.target
	p.target++
	if p.target >= paths {
		p.target = 0
	}
	if src == srcRequest && kind == kindCrackle {
		e.subs = min(e.subs, shortCrackle)
	}
	g := p.kindGain[e.path][b2i(src == srcRequest)][kind]
	a := scale * level * g
	ms := sh.bodyMS
	if ms == 0 {
		ms = p.bodyMS
	}
	ms *= 0.75 + 0.5*float64(p.draw())
	spread := 0.15
	if src == srcRequest {
		spread = 0.08
	}
	hz := p.bodyHz * sh.bodyRatio * (1 - spread + 2*spread*float64(p.draw()))
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
	e.clickLvl, e.gapZ = 0, 0
	if sh.click {
		e.clickLvl, e.crackLvl = e.crackLvl, 0
	}
	e.kind, e.src, e.start = kind, src, p.now
	e.spacing = float32(sh.spacingMS*1e-3) * p.rate
	e.next = p.subSpacing(e)
	if sh.click {
		e.spacing *= p.gapMean
	}
	// The slot holds the train and six body decays, down by 120 dB.
	e.left = max(p.busyLen, int(float32(e.subs)*1.6*e.spacing)+int(6*ms*1e-3*float64(p.rate)))
	p.active++
	p.count[kind]++
	p.total++
	p.lastKind, p.lastSrc, p.lastStrength = kind, src, strength
	p.last = p.now
	return e
}

// subSpacing returns the samples to the next sub-event, from 0.65 to 1.55
// times the median spacing. A click gap is log-normal, from the normal
// value gapZ that follows the one before with the correlation gapRho. The
// sum of three uniform values from -1 to 1 is about normal, with a
// variance of 1.
func (p *pops) subSpacing(e *popEvent) int {
	if !shapes[e.kind].click {
		return max(1, int(e.spacing*(0.65+0.9*p.subDraw())))
	}
	n := 2*(p.subDraw()+p.subDraw()+p.subDraw()) - 3
	e.gapZ = gapRho*e.gapZ + p.gapInnov*n
	i := min(len(p.gapTab)-1, max(0, int((e.gapZ+gapRange)*gapSteps+0.5)))
	return max(1, int(gapMS*1e-3*p.rate*p.gapTab[i]))
}

// bang is the bang of unburnt fuel in an ignition cut: a pop with a
// shorter pressure jump, so that its attack is sharper.
func (p *pops) bang(paths int) {
	level := 1 - 0.65*p.noise.Unipolar()
	if e := p.fire(paths, kindPop, srcLimiter, p.gain*bangGain*p.heatFactor()*p.level, level, level); e != nil {
		e.jumpLen = max(1, e.jumpLen/3)
		e.jumpLeft = e.jumpLen
	}
}

// tick runs once per sample. It sets the pressure jump of each path in
// p.jump and returns the crack for the outlet.
func (p *pops) tick() (crack float32) {
	if p.active == 0 {
		return 0
	}
	p.sounded = true
	p.jump = [MaxBanks]float32{}
	var env, clicks float32
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
				u := 0.3 + 0.7*p.subDraw()
				e.jumpLeft = e.jumpLen
				e.jumpAmp = e.amp * sh.subGain * u
				if sh.click {
					e.clickLvl = max(e.clickLvl, e.amp*p.crackRatio*sh.subCrack*u)
				} else {
					e.crackLvl += e.amp * p.crackRatio * sh.subCrack * u
				}
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
		e.clickLvl *= p.clickDec
		clicks += e.clickLvl
	}
	n := p.noise.Bipolar()
	crack = p.crackHP.Process(n)*env + p.clickBP.Process(n)*clicks
	if p.active == 0 {
		p.jump = [MaxBanks]float32{}
	}
	return crack
}
