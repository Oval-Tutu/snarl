package core

import (
	"math"

	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// golden is the step of the low-discrepancy sequence for the pop
	// intervals inside a lift-off run.
	golden = 0.6180339887
	// bangGain is the gain of a limiter bang relative to a pop.
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
)

// The pop timing in seconds, chances and gates.
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
// same span, both before the leveller. ownNorm brings the median level of
// an own pop, 0.675, to 1, so that the median own pop has its target.
var overDB = [numKinds]float64{kindPop: 4.8, kindBoom: 4.5, kindCrackle: 1.5, kindBurble: -4.3}

const ownNorm = 1 / 0.675

// The crackle sub-event gaps are log-normal around gapMS with the spread
// gapSigma (docs/research/crackles.md section 11). They set the length of
// the slot of a crackle.
const (
	gapMS    = 6.5
	gapSigma = 0.95
)

// The pop limiter keeps the engine and the pops below popCeiling of full
// scale. Its gain recovers with the time constant popReleaseSec.
const (
	popCeiling    = 0.95
	popReleaseSec = 0.05
)

// The pop kinds, from the clusters of the muffler crackle pack in
// docs/research/crackles.md. The snarl PopKind values are these plus 1.
const (
	kindPop     = iota // An isolated pop with a resonant body.
	kindBoom           // A short deep burst.
	kindCrackle        // A long train of short jumps over a deep body.
	kindBurble         // A soft low pop.
	numKinds
)

// The pop sources, with the values of the snarl PopSource type.
const (
	srcRequest = iota + 1 // A pop that the game requests.
	srcOwn                // A pop of a lift-off or a wind-down.
	srcLimiter            // A limiter bang.
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

// popShape is the length of one pop kind. A pop is a train of sub-events:
// the first starts the pop, and subs more follow at about spacingMS. The
// sub-events and the body decay set the length of the slot of the pop and
// the length control of its voice (voiceOptions).
type popShape struct {
	bodyMS    float64 // Body decay to -20 dB, or 0 for the profile body_ms.
	subs      int     // Least number of later sub-events.
	subsRange int     // Spread of the number of later sub-events.
	spacingMS float64 // Median spacing of the sub-events.
	logNormal bool    // Sub-events follow at log-normal gaps.
}

// shapes holds the length of each kind. The body decays, sub-event counts
// and spacings are the medians of the pack clusters.
var shapes = [numKinds]popShape{
	kindPop:     {},
	kindBoom:    {bodyMS: 50, subs: 3, subsRange: 12, spacingMS: 12.5},
	kindCrackle: {bodyMS: 70, subs: 5, subsRange: 27, spacingMS: gapMS, logNormal: true},
	kindBurble:  {bodyMS: 34, subs: 0, subsRange: 6, spacingMS: 11},
}

// voiceOptions returns the options of the crackle model voice of a pop of
// the kind with subs later sub-events, at the level 0 dB. The mapping and
// its sources are in pops.sources.toml. A crackle is a train whose active
// length has the rank in the measured lengths of m that the sub-event
// count has in its range. A boom is a short train of one shock per
// sub-event. A pop and a burble are the single-shock mode.
func voiceOptions(m *crackle.Params, kind, subs int) crackle.Options {
	switch kind {
	case kindCrackle:
		sh := &shapes[kind]
		r := 4 * float64(subs-sh.subs) / float64(sh.subsRange)
		i := min(int(r), 3)
		l := &m.LengthMS
		return crackle.Options{LengthMS: l[i] + (r-float64(i))*(l[i+1]-l[i])}
	case kindBoom:
		return crackle.Options{Shocks: 1 + subs, Short: true}
	}
	return crackle.Options{Single: true}
}

// popEvent is the slot of one sounding pop.
type popEvent struct {
	left      int // Steps until the slot is free.
	kind, src int
	start     int64 // Start time in steps.
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
	n         int
}

// pops is the pop model: the requested pops, the own pops of a lift-off
// or a wind-down, the limits of the own pops and the pop events. All
// triggers and the slots run once per control block. Each slot plays its
// pop on a voice of the crackle model, which Engine.mix adds to the
// synthesiser output after the leveller.
type pops struct {
	density    float32 // Multiplier of the own pop chances.
	bangChance float32
	gain       float32
	rate       float32
	character  int
	liftRPM    float32

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

	// Times in steps. now is the start of the control block, and last the
	// start of the last pop. The other times are the lengths of the timing
	// constants.
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
	cum             [numCharacters][2][numKinds]float32 // Run kind weights: first pop, later pops.
	gainOwn, gainRq [numCharacters]float32

	// Report: pops started, and the kind, source and strength of the last.
	total        uint32
	lastKind     int
	lastSrc      int
	lastStrength float32

	// Events. Slot i plays on voices[i].
	ev      [maxPopEvents]popEvent
	voices  [maxPopEvents]*crackle.Voice
	active  int
	busyLen int
	bodyMS  float64
	gapMean float32 // Mean gap over the median, exp(gapSigma^2/2).
	model   *popModel
}

// newPops returns the pop model at the simulation step rate, with voices
// of the shared crackle model m.
func newPops(s *Spec, m *popModel, rate, chunkSeconds float64) pops {
	p := s.Pops
	ch, _ := characterIndex(p.Character)
	q := pops{
		density:   float32(densityScale * p.Density),
		gain:      float32(p.Gain),
		rate:      float32(rate),
		character: ch,
		liftRPM:   float32(liftRedline * s.RedlineRPM),
		noise:     dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x9e37),
		heatUp:    dsp.TimeCoef(3, 1/chunkSeconds),
		heatDown:  dsp.TimeCoef(15, 1/chunkSeconds),
		guard:     int64(guardSec * rate),
		gap:       int64(gapSec * rate),
		window:    int64(ownSeconds * rate),
		reqKind:   -1,
		histLen:   int(histSeconds * rate),
		busyLen:   int(0.3 * rate),
		bodyMS:    p.BodyMS,
		gapMean:   float32(math.Exp(gapSigma * gapSigma / 2)),
		model:     m,
	}
	// The seeds of the voices come from the pop random source, so that the
	// engine sound does not depend on the pops.
	for i := range q.voices {
		q.voices[i] = crackle.New(m.bank, q.noise.Next())
	}
	if p.LimiterBangs {
		q.bangChance = float32(2 * p.Density)
	}
	if span := float64(q.liftRPM) - windIdle*s.IdleRPM; span > 0 {
		q.windBands = max(1, int(math.Round(span/windBandRPM)))
		q.windInv = float32(float64(q.windBands) / span)
	}
	// The engine starts below the bands, so that the RPM rising from 0
	// rolls no band.
	q.windIn = q.windBands
	q.last = -q.guard
	for i := range q.ownTimes {
		q.ownTimes[i] = -q.window
	}
	q.lastOwn = -q.gap
	w := [numKinds]float64{p.WeightPop, p.WeightBoom, p.WeightCrackle, p.WeightBurble}
	for c, cs := range characters {
		q.cum[c][0] = cumulative(w, cs.first)
		q.cum[c][1] = cumulative(w, cs.later)
		q.gainOwn[c] = float32(math.Pow(10, cs.gainDB/20))
		q.gainRq[c] = float32(math.Pow(10, cs.requestDB/20))
	}
	return q
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

// sec returns seconds in steps.
func (p *pops) sec(s float64) int64 {
	return int64(s * float64(p.rate))
}

// uniform returns a random time in steps from lo to hi seconds.
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
		p.requested(ch)
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
			p.own(ch, pick(p.noise.Unipolar(), &p.cum[ch][pos]), trigLift, now)
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
		p.own(ch, k, trigWind, now)
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

// history adds the throttle of a block of n steps to the throttle
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
func (p *pops) own(ch, kind, trig int, now int64) {
	level := 1 - 0.65*p.noise.Unipolar()
	if !p.fire(kind, srcOwn, p.gain*p.gainOwn[ch]*p.heatFactor()*ownNorm, level, level) {
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
func (p *pops) requested(ch int) {
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
	scale := p.gain * p.gainRq[ch]
	level := float32(math.Pow(10, 0.15*float64(p.noise.Bipolar())))
	p.fire(k, srcRequest, scale, level, s)
	p.reqPending--
}

// b2i returns 1 for true and 0 for false.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// heatFactor maps the wall heat to a pop gain. A cold exhaust cannot pop.
func (p *pops) heatFactor() float32 {
	return min(1, max(0, (p.heat-0.08)*4))
}

// nextInterval returns the steps to the next pop of a run, from runGapMin
// to runGapMax seconds.
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

// slot returns the index of the slot for a pop of the source. It is a
// free slot, or for a requested pop the slot of the oldest own pop, or of
// the oldest pop when no own pop sounds. Otherwise it is -1.
func (p *pops) slot(src int) int {
	oldOwn, old := -1, -1
	for i := range p.ev {
		e := &p.ev[i]
		if e.left == 0 {
			return i
		}
		if e.src == srcOwn && (oldOwn < 0 || e.start < p.ev[oldOwn].start) {
			oldOwn = i
		}
		if old < 0 || e.start < p.ev[old].start {
			old = i
		}
	}
	if src != srcRequest {
		return -1
	}
	if oldOwn >= 0 {
		old = oldOwn
	}
	p.ev[old].left = 0
	p.active--
	return old
}

// fire starts one pop of a kind from the source on the voice of a slot,
// and reports false when no slot is free. The gain is scale times level
// times the kind gain, over an engine of unit level, so that each kind
// keeps its level over the engine. strength is the strength in the pop
// report. The body decay spreads over plus or minus 25 per cent.
func (p *pops) fire(kind, src int, scale, level, strength float32) bool {
	i := p.slot(src)
	if i < 0 {
		return false
	}
	e := &p.ev[i]
	sh := &shapes[kind]
	subs := sh.subs + int(float32(sh.subsRange+1)*p.noise.Unipolar())
	if src == srcRequest && kind == kindCrackle {
		subs = min(subs, shortCrackle)
	}
	ms := sh.bodyMS
	if ms == 0 {
		ms = p.bodyMS
	}
	ms *= 0.75 + 0.5*float64(p.noise.Unipolar())
	spacing := float32(sh.spacingMS*1e-3) * p.rate
	if sh.logNormal {
		spacing *= p.gapMean
	}
	// The slot holds the train and six body decays, down by 120 dB, and
	// at least until its voice ends (tick).
	e.left = max(p.busyLen, int(float32(subs)*1.6*spacing)+int(6*ms*1e-3*float64(p.rate)))
	e.kind, e.src, e.start = kind, src, p.now
	o := voiceOptions(p.model.params, kind, subs)
	o.LevelDB = 20 * math.Log10(float64(scale*level*p.model.gain[b2i(src == srcRequest)][kind]))
	p.voices[i].Trigger(o)
	p.active++
	p.total++
	p.lastKind, p.lastSrc, p.lastStrength = kind, src, strength
	p.last = p.now
	return true
}

// bang is the bang of unburnt fuel in a limiter cut: the single shock of
// a pop at bangGain.
func (p *pops) bang() {
	level := 1 - 0.65*p.noise.Unipolar()
	p.fire(kindPop, srcLimiter, p.gain*bangGain*p.heatFactor(), level, level)
}

// tick runs once per control block, after its n simulation steps. It
// frees the slots whose time has passed and whose voice has ended.
func (p *pops) tick(n int) {
	if p.active == 0 {
		return
	}
	for i := range p.ev {
		e := &p.ev[i]
		if e.left == 0 {
			continue
		}
		e.left = max(e.left-n, 0)
		if e.left == 0 && p.voices[i].Active() {
			e.left = 1
		}
		if e.left == 0 {
			p.active--
		}
	}
}
