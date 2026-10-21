package model

import (
	"errors"
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

const (
	// chunk is the control period in samples. Smoothed controls and
	// block-rate coefficients update once per chunk.
	chunk = 32
	// pulseLen is the length of one pulse in envelope steps.
	pulseLen = 512
	// maxRPM bounds the input RPM.
	maxRPM = 20000
	// noiseScale brings the 2 kHz low-passed noise to about unit range.
	noiseScale = 3
	// gateStep is the change of the limiter jet noise gate per sample.
	gateStep = 1.0 / 16
	// notchLen is the length in seconds of the limiter notch at the start
	// of a cut.
	notchLen = 0.07
	// barkDecay is the time constant in seconds of the re-fire bark.
	barkDecay = 0.01
	// handover is the time constant in seconds over which the pitch
	// returns from the internal limiter RPM to the smoothed RPM.
	handover = 0.017
	// fadeDB is the rise in decibels per second of the firing gain after
	// the final gear ends a cut. A fixed rate in decibels gives no step,
	// for a shallow or a deep notch.
	fadeDB = 200
	// engLevelSec is the time constant in seconds of the engine level
	// that the pops follow.
	engLevelSec = 0.05
	// waverHz1 and waverHz2 are the rates of the two slow sines of the
	// final gear waver. Their ratio is not a simple fraction, so that the
	// waver does not repeat audibly.
	waverHz1 = 0.37
	waverHz2 = 1.13
)

// controlBlock is the control period in samples that New and Render use,
// from 1 to chunk. It is chunk except in a test that measures the stepping
// of the block-rate controls.
var controlBlock = chunk

// wholeSampleStart is true only in a test that compares the pulses with
// pulses that start on the whole sample at or after their position, with
// no band-limited corner and no end at zero.
var wholeSampleStart bool

// Control is the engine state for one block.
type Control struct {
	RPM, Throttle, Load float32
	Running, Overrun    bool
	// Final is true in the final gear. The limiter does not cut, and the
	// engine holds the limiter RPM.
	Final bool
	// LimiterEvents is the number of rev limiter events since the last
	// block.
	LimiterEvents int
	// Gear is the gear of the snapshot. A change ends a limiter bounce.
	Gear uint8
	// PopRequests is the number of pops that the game requests since the
	// last block, and PopStrength the strength of the latest, from 0 to 1.
	PopRequests int
	PopStrength float32
	// NoPops stops the own pops of a lift-off or a wind-down.
	NoPops bool
	// Character is the pop character: 0 for the profile character, and 1,
	// 2 and 3 for stock, sport and race.
	Character int
}

type voice struct {
	pos, gain float32
	e1, e2    float32
	bank      int
	active    bool
	started   bool
}

// Synth renders one engine. It allocates nothing after New.
type Synth struct {
	rate float64

	noise, noise2 dsp.Noise
	idle          idle
	crank         crank
	voices        []voice
	delay         []float32
	cylGain       []float32
	cylDev        []float32
	stepPerCycle  float32
	maxInc        float32
	invRate120    float32

	banks int
	mix   [MaxBanks][MaxBanks]float32
	paths []path
	pops  pops
	out   output

	// Profile constants.
	cycleVar, jitter            float32
	redline, camRPM             float32
	camGain, camAttack, attack  float32
	decay                       float32
	amCfg, jetCfg               float32
	steepCfg, voiceGain         float32
	overlap                     float32
	limKind                     string
	limRPM, limFall, limRise    float32
	limCut                      int
	limJitter                   float32
	pumpLvl, notchLvl, barkLvl  float32
	notchStep, kBark            float32
	kRPM, kCtl, kMaster, kCam   float32
	kLim, kHand                 float32
	jit                         dsp.OnePole
	idleRPM, ovLevel, ovIdle    float32
	partLoad                    float32
	idleGain, knee              float32
	rpmG, rpmStep               float32
	shape                       float32
	jetBP                       dsp.Biquad
	rpm, throttle, load, master float32
	overrun, camMix             float32
	camHigh                     bool
	inc, step, amp, amDepth     float32
	jetGain, ka, kb, liveNorm   float32

	// Pulse placement for one chunk. aStep and bStep are the attack and
	// decay rates per pulse step, so that a voice starts its envelope at
	// its fractional position. endVal is the envelope at the end of the
	// pulse, which the pulse subtracts along its length, so that it ends
	// at zero. corner is the slope of the pulse at its start per sample,
	// times liveNorm/6, the gain of the band-limited corner. invStep is
	// 1/step.
	aStep, bStep, endVal float32
	corner, invStep      float32

	// amLP and amLP2 are the AM noise low pass. Their coefficients move
	// with the RPM from amIdleA and 1 at idle to amTopA and amTopA2 at the
	// redline. amIdle is the AM depth factor at idle, and amTaper is its
	// fall from idle to the redline.
	amLP, amLP2              dsp.OnePole
	amIdleA, amTopA, amTopA2 float32
	amIdle, amTaper          float32

	// Engine level for the pops. engS and engE sum the output and its
	// square over the chunk, before the RPM gain and the output stage,
	// which the pops also pass. engMean and engMS follow their means over
	// the chunks with no pop with the coefficient kEng, and hold while a
	// pop sounds. The level is the RMS level with no DC, which the output
	// stage removes.
	engS, engE, engMean, engMS, kEng float32

	// Rev limiter state. cut is the samples left of the cut, and cutLen is
	// the length of the cut in samples. cutting is
	// true for a chunk inside a cut. bounce is true while the snapshot
	// holds the engine at the limiter, and climb is true while the
	// internal RPM lrpm climbs back to the limiter RPM after a cut of a
	// bounce. dip is the pitch offset below the smoothed RPM. limRot
	// counts the cuts. bangAt is the time into an ignition cut in samples
	// at which the unburnt fuel bangs, or -1. gate is the jet noise gain of
	// the cut, and gateTo is its target. gear is the last gear. pump is
	// the gain of a pumping pulse in a cut, notch is the phase of the notch
	// in cycles, and bark is the extra firing gain after a cut. fade is the
	// firing gain after the final gear ends a cut. It starts at the pump
	// level and rises by the factor fadeRate per chunk to 1.
	cut, cutLen, limRot     int
	bangAt                  int
	cutting, limEvent       bool
	bounce, climb           bool
	gear                    uint8
	lrpm, dip, gate, gateTo float32
	pump, notch, bark       float32
	fade, fadeRate          float32

	// Final gear waver. waver is the depth from the profile, and wavMix
	// fades it in and out at the limiter RPM. wav is the pitch offset of
	// the chunk, from -waver to 0. wavPh1 and wavPh2 are the sine phases in
	// cycles, which advance by wavInc1 and wavInc2 per chunk.
	waver, wavMix, wav float32
	wavPh1, wavPh2     float32
	wavInc1, wavInc2   float32
}

// New returns a synth for the spec at the sample rate.
func New(s *Spec, rate int) (*Synth, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if rate < 8000 {
		return nil, errors.New("sample rate is below 8000 Hz")
	}
	fs := float64(rate)
	chunkHz := fs / float64(controlBlock)
	seed := uint32(s.Seed & math.MaxUint16)
	y := &Synth{
		rate:       fs,
		noise:      dsp.NewNoise(seed),
		noise2:     dsp.NewNoise(seed + 0x51ed),
		invRate120: float32(1 / (120 * fs)),
		banks:      s.Banks(),
		cycleVar:   float32(s.Pulse.CycleVariation),
		jitter:     float32(s.Pulse.Jitter),
		redline:    float32(s.RedlineRPM),
		camRPM:     float32(s.CamSwitchRPM),
		camGain:    float32(s.Pulse.CamGain),
		attack:     float32(s.Pulse.Attack),
		camAttack:  float32(s.Pulse.CamAttack),
		decay:      float32(s.Pulse.Decay),
		amCfg:      float32(s.Noise.AM),
		jetCfg:     float32(s.Noise.Jet),
		steepCfg:   float32(s.Exhaust.Steepening),
		overlap:    float32(s.Exhaust.Overlap),
		voiceGain:  float32(s.Voice.Gain),
		limKind:    s.LimiterKind,
		limRPM:     float32(s.LimiterRPM),
		limFall:    float32(s.LimiterFallRPMS * s.LimiterLoadScale / fs),
		limRise:    float32(s.LimiterRiseRPMS * s.LimiterLoadScale / fs),
		limCut:     int(s.LimiterMS * 1e-3 * fs),
		limJitter:  float32(s.LimiterJitter),
		fade:       1,
		fadeRate:   float32(math.Pow(10, fadeDB/20/chunkHz)),
		waver:      float32(s.FinalGearWaver),
		wavInc1:    float32(waverHz1 / chunkHz),
		wavInc2:    float32(waverHz2 / chunkHz),
		pumpLvl:    float32(math.Pow(10, s.LimiterPumpDB/20)),
		notchLvl:   float32(math.Pow(10, s.LimiterNotchDB/20)),
		barkLvl:    float32(math.Pow(10, s.LimiterBarkDB/20) - 1),
		notchStep:  float32(1 / (notchLen * fs)),
		kBark:      1 - dsp.TimeCoef(barkDecay, chunkHz),
		gate:       1,
		gateTo:     1,
		kRPM:       dsp.TimeCoef(0.012, chunkHz),
		kCtl:       dsp.TimeCoef(0.025, chunkHz),
		kMaster:    dsp.TimeCoef(0.02, chunkHz),
		kCam:       dsp.TimeCoef(0.015, chunkHz),
		kLim:       dsp.TimeCoef(0.002, chunkHz),
		kHand:      1 - dsp.TimeCoef(handover, chunkHz),
		kEng:       dsp.TimeCoef(engLevelSec, chunkHz),
		jit:        dsp.OnePole{A: dsp.OnePoleCoef(2, chunkHz)},
		amIdleA:    dsp.OnePoleCoef(2000, fs),
		amTopA:     dsp.OnePoleCoef(2000, fs),
		amTopA2:    1,
		amIdle:     float32(s.Noise.AMIdle),
		amTaper:    float32(s.Noise.AMIdle - s.Noise.AMRedline),
		idleRPM:    float32(s.IdleRPM),
		ovLevel:    float32(math.Pow(10, s.Overrun.LevelDB/20)),
		ovIdle:     float32(math.Pow(10, s.Overrun.IdleLevelDB/20)),
		partLoad:   float32(math.Pow(10, s.Pulse.PartLoadDB/20)),
		rpmG:       -1,
		pops:       newPops(s, fs, float64(controlBlock)/fs),
		out:        newOutput(s.Voice.OutputDB, fs),
		idleGain:   float32(s.Voice.IdleGainDB),
	}
	if s.Noise.AMCutoffHz > 0 {
		y.amTopA = dsp.OnePoleCoef(s.Noise.AMCutoffHz, fs)
		y.amTopA2 = y.amTopA
	}
	y.knee = float32(max(s.Voice.IdleGainRPM, s.IdleRPM))
	if y.camAttack <= 0 {
		y.camAttack = y.attack
	}
	jetHz := s.Noise.JetHz
	if jetHz <= 0 {
		jetHz = 1500
	}
	y.jetBP.SetBandPass(jetHz, 0.7, fs)

	y.idle = newIdle(s, seed, chunkHz)
	y.initCylinders(s, fs)
	y.initPaths(s, fs)
	y.pops.calibrateCached(s, fs)
	return y, nil
}

func (y *Synth) initCylinders(s *Spec, fs float64) {
	n := len(s.Cylinders)
	y.voices = make([]voice, n)
	y.delay = make([]float32, n)
	y.cylGain = make([]float32, n)
	y.cylDev = make([]float32, n)
	offsets := make([]float32, n)
	setup := dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x7a11)
	for i, c := range s.Cylinders {
		deg := c.CrankDeg + s.Pulse.TimingVariationDeg*float64(setup.Bipolar())
		deg = math.Mod(deg+720, 720)
		y.cylDev[i] = setup.Bipolar()
		y.cylGain[i] = 1 + float32(s.Pulse.GainVariation)*y.cylDev[i]
		offsets[i] = float32(deg / 720)
		y.voices[i].bank = c.Bank
		y.delay[i] = float32(c.PrimaryM / s.Exhaust.SpeedOfSound * fs)
	}
	y.crank = newCrank(offsets)
	ev := y.crank.events
	gap := 1 - ev[n-1].offset + ev[0].offset
	for i := 1; i < n; i++ {
		gap = min(gap, ev[i].offset-ev[i-1].offset)
	}
	y.maxInc = 0.9 * gap

	width := s.Pulse.WidthDeg / 720
	y.stepPerCycle = float32(pulseLen / width)
}

func (y *Synth) initPaths(s *Spec, fs float64) {
	nb := y.banks
	var k float64
	switch s.Exhaust.Crossover {
	case "full":
		k = 1 - 1/float64(nb)
	case "partial":
		k = s.Exhaust.Coupling
	}
	for p := range nb {
		for b := range nb {
			switch {
			case nb == 1:
				y.mix[p][b] = 1
			case p == b:
				y.mix[p][b] = float32(1 - k)
			default:
				y.mix[p][b] = float32(k / float64(nb-1))
			}
		}
	}
	y.paths = make([]path, nb)
	for i, ps := range s.Paths {
		y.paths[i] = newPath(ps, s.Exhaust, fs)
	}
}

// envelopePeak returns the peak of (1-exp(-a x))exp(-b x) for x >= 0.
func envelopePeak(a, b float64) float64 {
	x := math.Log((a+b)/b) / a
	return (1 - math.Exp(-a*x)) * math.Exp(-b*x)
}

// PitchRPM returns the RPM that the firing follows. It differs from the
// smoothed input RPM in a limiter bounce, in the handover after it, and in
// the final gear waver.
func (y *Synth) PitchRPM() float32 {
	return max(0, (y.rpm-y.dip)*(1+y.wav))
}

// PopReport returns the number of pops started since New, and the kind,
// source and strength of the latest. The kinds count from 0 and the
// sources from 1, in the order of the snarl PopKind and PopSource values.
func (y *Synth) PopReport() (count uint32, kind, source int, strength float32) {
	p := &y.pops
	return p.total, p.lastKind, p.lastSrc, p.lastStrength
}

// Render fills dst. It allocates nothing.
func (y *Synth) Render(dst []float32, c Control) {
	if c.LimiterEvents > 0 && !c.Final {
		y.limEvent = true
	}
	if c.PopRequests > 0 {
		y.pops.request(c.PopRequests, c.PopStrength)
	}
	for len(dst) > 0 {
		n := min(len(dst), controlBlock)
		y.control(&c, n)
		y.process(dst[:n])
		dst = dst[n:]
	}
}

func clampf(x, lo, hi float32) float32 {
	if x != x { // NaN.
		return lo
	}
	return min(hi, max(lo, x))
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// loadLevel returns the pulse gain at load ld, relative to full load at
// the same RPM. It rises with load along lvl+(1-lvl)*ld^k, from the
// overrun level lvl at load 0 to 1 at load 1. The exponent k puts the gain
// at load 0.5 on part. When lvl is not below part, k stops at its upper
// bound, and the gain stays near lvl up to high load.
func loadLevel(ld float64, lvl, part float32) float32 {
	k := math.Log2(float64(1-lvl) / math.Max(float64(part-lvl), 1e-3))
	return lvl + (1-lvl)*float32(math.Pow(ld, min(max(k, 0.2), 8)))
}

// rpmGain returns the output gain at the RPM for an extra gain of db
// decibels at and below the knee RPM. Above the knee, the gain in decibels
// falls along a smoothstep to 0 at the redline.
func rpmGain(rpm, knee, redline, db float32) float32 {
	if db == 0 {
		return 1
	}
	w := min(1, max(0, (redline-rpm)/(redline-knee)))
	return float32(math.Pow(10, float64(db*w*w*(3-2*w))/20))
}

// control runs once per chunk of n samples.
func (y *Synth) control(c *Control, n int) {
	rpm := clampf(c.RPM, 0, maxRPM)
	if c.Final && y.limRPM > 0 {
		// The final gear runs out of power at the limiter RPM.
		rpm = min(rpm, y.limRPM)
	}
	thr := clampf(c.Throttle, 0, 1)
	load := clampf(c.Load, 0, 1)
	y.rpm += (rpm - y.rpm) * y.kRPM
	y.throttle += (thr - y.throttle) * y.kCtl
	y.load += (load - y.load) * y.kCtl
	y.master += (b2f(c.Running) - y.master) * y.kMaster
	limiting := y.limit(rpm, thr, c.Gear, c.Final, n)
	over := c.Overrun && thr < 0.05 && !limiting
	k := y.kCtl
	if limiting {
		k = y.kLim
	}
	y.overrun += (b2f(over) - y.overrun) * k

	if y.camRPM > 0 {
		if !y.camHigh && y.rpm > y.camRPM {
			y.camHigh = true
		} else if y.camHigh && y.rpm < y.camRPM-100 {
			y.camHigh = false
		}
		y.camMix += (b2f(y.camHigh) - y.camMix) * y.kCam
	}

	j := y.jit.Process(y.noise.Bipolar() * 1.7)
	y.idle.control(rpm, y.load)
	speed := y.invRate120 * (1 + y.jitter*j) * y.idle.speed
	y.inc = min(y.PitchRPM()*speed, y.maxInc)
	y.step = y.inc * y.stepPerCycle

	ld := float64(y.load)
	rr := min(y.rpm/y.redline, 1.2)
	// The pulse gain follows the load curve. On overrun, it moves to the
	// overrun level.
	pos := min(1, max(0, (y.rpm-y.idleRPM)/(y.redline-y.idleRPM)))
	lvl := y.ovIdle + (y.ovLevel-y.ovIdle)*pos
	shape := loadLevel(ld, lvl, y.partLoad)
	y.shape = shape + (lvl-shape)*y.overrun
	y.amp = y.master * y.voiceGain * y.shape * (0.5 + 0.5*rr) *
		(1 + y.camGain*y.camMix) * (1 + 2*y.jitter*j) * y.idle.rock
	gate := float32(math.Pow(math.Max(ld, 0.02), 0.7))
	// Towards the redline, the AM noise gets narrower and shallower along
	// a smoothstep, so that the harmonics rise above it. The depth moves
	// from its idle factor to its redline factor.
	w := pos * pos * (3 - 2*pos)
	y.amDepth = y.amCfg * gate * (y.amIdle - y.amTaper*w)
	y.amLP.A = y.amIdleA + (y.amTopA-y.amIdleA)*w
	y.amLP2.A = 1 + (y.amTopA2-1)*w

	// The RPM gain ramps over the chunk. It starts at its first value.
	g := rpmGain(y.rpm, y.knee, y.redline, y.idleGain)
	if y.rpmG < 0 {
		y.rpmG = g
	}
	y.rpmStep = (g - y.rpmG) / float32(n)

	heat := b2f(y.throttle > 0.2) * y.load * min(rr, 1)
	y.pops.control(&popInput{
		rpm: y.rpm, throttle: thr, overrun: over && y.master > 0.5, cut: limiting, off: c.NoPops,
		heat: heat, character: c.Character, level: float32(math.Sqrt(math.Max(0, float64(y.engMS-y.engMean*y.engMean)))),
		n: n, paths: len(y.paths),
	})
	// The exhaust flow layers follow the pulse gain, so that they keep the
	// level order over load.
	y.controlLayers(y.shape, rr)
}

// limit runs the rev limiter for a chunk of n samples, from the input RPM,
// throttle and gear. It follows the engine-sim limiter: a cut stops the
// firing of every cylinder for the cut length while the internal RPM falls
// at the fall rate. After the cut, the engine fires and the internal RPM
// climbs at the rise rate to the limiter RPM, where the next cut starts.
// The bounce runs while the snapshot RPM is at the limiter with the
// throttle open. A limiter event starts one cut when no cut or climb runs.
// When the bounce or an event cut ends, the pitch returns to the smoothed
// RPM. It reports whether a cut or a climb runs, so that no overrun effect
// plays.
//
// In a cut, each cylinder pumps a pulse at the pump level. The level dips
// to the notch level shortly after the cut starts, and the first firings
// after the cut bark. The engine-sim output has this level shape, from its
// leveller.
//
// In the final gear, no cut starts, and a cut or a climb in progress ends.
// The firing gain returns from the pump level to full with the pitch, with
// no bark. At the limiter RPM with the throttle open, the pitch wavers
// slowly below the limiter RPM, so that the held tone does not sound
// fixed.
func (y *Synth) limit(rpm, thr float32, gear uint8, final bool, n int) bool {
	was := y.cutting
	y.bark *= y.kBark
	y.waverStep(final && y.limRPM > 0 && rpm >= y.limRPM*0.998 && thr > 0.1)
	// The smoothed RPM must be near the limiter too, so that a bounce
	// starts from the limiter RPM.
	at := !final && y.limRPM > 0 && rpm >= y.limRPM*0.998 && y.rpm >= y.limRPM*0.99 && thr > 0.1 && gear == y.gear
	y.gear = gear
	if final && y.cut > 0 {
		y.fade = y.pump
		y.bark = 0
	}
	if y.bounce && !at || final {
		y.bounce, y.climb, y.cut = false, false, 0
	}
	if !final && y.cut <= 0 && !y.climb && (at || y.limEvent) && y.limCut > 0 {
		// The draw only runs with a jitter, so that a profile with none
		// keeps its random sequence.
		y.cutLen = y.limCut
		if y.limJitter > 0 {
			y.cutLen = int(float32(y.limCut) * (1 + y.limJitter*y.noise.Bipolar()))
		}
		y.cut = y.cutLen
		y.bounce = at
		y.lrpm = y.rpm - y.dip
		y.notch = 0
		y.fade = 1
		y.limRot++
		y.bangAt = -1
		// An ignition cut bangs with a chance of twice the pop density,
		// at a random time in the cut.
		if y.limKind == "ignition" && y.noise.Unipolar() < y.pops.bangChance {
			y.bangAt = int((0.2 + 0.8*y.noise.Unipolar()) * float32(y.cutLen))
		}
	}
	y.limEvent = false
	y.cutting = y.cut > 0
	if was && !y.cutting && !final {
		y.bark = y.barkLvl
	}
	y.gateTo = 1
	switch {
	case y.cutting:
		y.cut -= n
		y.lrpm -= y.limFall * float32(n)
		y.climb = y.cut <= 0 && y.bounce
		// The notch is a raised cosine over notchLen.
		y.pump = y.pumpLvl
		if y.notch < 1 {
			y.pump += (y.notchLvl - y.pumpLvl) * (0.5 - 0.5*dsp.Sine(y.notch+0.25))
			y.notch += y.notchStep * float32(n)
		}
		y.gateTo = y.pump
	case y.climb:
		y.lrpm += y.limRise * float32(n)
		if y.lrpm >= y.limRPM {
			y.lrpm = y.limRPM
			y.climb = false
		}
	default:
		// Handover: the offset decays, so that the pitch returns to the
		// smoothed RPM with no step.
		y.dip *= y.kHand
		if y.dip < 1 && y.dip > -1 {
			y.dip = 0
		}
		if y.fade < 1 {
			y.fade = min(1, max(y.fade, 1e-3)*y.fadeRate)
			y.gateTo = y.fade
		}
		return false
	}
	y.dip = y.rpm - y.lrpm
	return true
}

// waverStep updates the final gear waver for one chunk. top is true in the
// final gear at the limiter RPM with the throttle open. The waver fades in
// and out with the control time constant. It is the sum of two slow table
// sines, mapped from -waver to 0, so that the pitch never passes the
// limiter RPM.
func (y *Synth) waverStep(top bool) {
	if y.waver == 0 || !top && y.wavMix == 0 {
		return
	}
	y.wavMix += (b2f(top) - y.wavMix) * y.kCtl
	if !top && y.wavMix < 1e-4 {
		y.wavMix, y.wav = 0, 0
		return
	}
	y.wavPh1 += y.wavInc1
	if y.wavPh1 >= 1 {
		y.wavPh1--
	}
	y.wavPh2 += y.wavInc2
	if y.wavPh2 >= 1 {
		y.wavPh2--
	}
	w := 0.6*dsp.Sine(y.wavPh1) + 0.4*dsp.Sine(y.wavPh2)
	y.wav = y.waver * y.wavMix * (w - 1) * 0.5
}

// controlLayers sets the pulse envelope coefficients, the wave steepening
// and the gain of the jet layer for one chunk.
func (y *Synth) controlLayers(flow, rr float32) {
	a := float64(y.attack + (y.camAttack-y.attack)*y.camMix)
	b := float64(y.decay)
	dx := float64(y.step) / pulseLen
	y.ka = float32(math.Exp(-a * dx))
	y.kb = float32(math.Exp(-b * dx))
	y.liveNorm = float32(1 / envelopePeak(a, b))
	y.aStep = float32(a / pulseLen)
	y.bStep = float32(b / pulseLen)
	y.endVal = float32((1 - math.Exp(-a)) * math.Exp(-b) / pulseLen)
	y.corner = float32(a*dx/6) * y.liveNorm
	y.invStep = 1 / max(y.step, 1e-9)
	if wholeSampleStart {
		y.aStep, y.bStep, y.endVal, y.corner = 0, 0, 0, 0
	}
	for i := range y.paths {
		y.paths[i].steep = y.steepCfg * flow
	}
	jet := float32(math.Pow(math.Max(float64(flow), 0.02), 0.7)) * rr
	y.jetGain = y.master * y.jetCfg * jet * jet
}

// fire starts the pulse of one cylinder. In a limiter cut, the pulse is a
// pumping pulse, with no combustion variation in gain or delay.
func (y *Synth) fire(cyl int) {
	var g, late float32
	if y.cutting {
		g = y.pump
		if y.bangAt >= 0 && y.cutLen-y.cut >= y.bangAt {
			y.pops.bang(len(y.paths))
			y.bangAt = -1
		}
	} else {
		g, late = y.idle.firing()
		g *= (1 + y.bark) * y.fade * (1 + y.cycleVar*y.noise.Bipolar())
	}
	v := &y.voices[cyl]
	v.gain = y.amp * (y.cylGain[cyl] + y.idle.imbalance*y.cylDev[cyl]) * g
	// The pulse starts one sample later than the crank places it, so that
	// the sample before its start, which carries part of the band-limited
	// corner, is still to come.
	lead := float32(1)
	if wholeSampleStart {
		lead = 0
	}
	v.pos = (y.crank.past-late)*y.stepPerCycle - (y.delay[cyl]+lead)*y.step
	v.active = true
	v.started = false
}

// pulses runs every active voice for one sample and sums the voices per
// bank.
//
// A pulse starts at its fractional position: its first sample is the
// envelope v.pos pulse steps after the start. The envelope starts at zero
// with a corner, a step of its slope. A two-sample polynomial band-limited
// ramp (polyBLAMP) rounds the corner: with d the fraction of a sample from
// the start to the first sample, the sample before the start adds
// corner*d^3 and the first sample adds corner*(1-d)^3. The pulse subtracts
// the envelope end value along its length, so that it ends at zero.
func (y *Synth) pulses(bank *[MaxBanks]float32) {
	step := y.step
	for k := range y.voices {
		v := &y.voices[k]
		if !v.active {
			continue
		}
		if v.pos < 0 {
			v.pos += step
			if v.pos >= 0 {
				d := v.pos * y.invStep
				bank[v.bank] += y.corner * d * d * d * v.gain
			}
			continue
		}
		if int(v.pos) >= pulseLen {
			v.active = false
			continue
		}
		var c float32
		if !v.started {
			v.started = true
			v.e1 = float32(math.Exp(float64(-y.aStep * v.pos)))
			v.e2 = float32(math.Exp(float64(-y.bStep * v.pos)))
			d := max(0, 1-v.pos*y.invStep)
			c = y.corner * d * d * d
		}
		e := ((1-v.e1)*v.e2-y.endVal*v.pos)*y.liveNorm + c
		v.e1 *= y.ka
		v.e2 *= y.kb
		bank[v.bank] += e * v.gain
		v.pos += step
	}
}

// process renders one chunk.
func (y *Synth) process(dst []float32) {
	np := len(y.paths)
	for i := range dst {
		if cyl := y.crank.step(y.inc); cyl >= 0 {
			y.fire(cyl)
		}
		var bank [MaxBanks]float32
		y.pulses(&bank)
		for b := range np {
			bank[b] += y.overlap * bank[b] * bank[b]
		}

		nz := y.amLP2.Process(y.amLP.Process(y.noise.Bipolar())) * noiseScale
		am := 1 + y.amDepth*nz
		crack := y.pops.tick()

		var sum float32
		for p := range np {
			var x float32
			for b := range np {
				x += y.mix[p][b] * bank[b]
			}
			sum += y.paths[p].process(x*am, y.pops.jump[p], y.pops.live[p])
		}
		sum += crack
		sum += y.jet()
		y.engS += sum
		y.engE += sum * sum
		y.rpmG += y.rpmStep
		dst[i] = y.out.process(sum * y.rpmG)
	}
	y.engineLevel(len(dst))
}

// engineLevel updates the engine level after a chunk of n samples. A
// chunk in which a pop sounded does not change it. The first chunk sets
// it.
func (y *Synth) engineLevel(n int) {
	inv := 1 / float32(n)
	mean, ms := y.engS*inv, y.engE*inv
	y.engS, y.engE = 0, 0
	switch {
	case y.pops.sounded:
		y.pops.sounded = false
	case y.engMS == 0:
		y.engMean, y.engMS = mean, ms
	default:
		y.engMean += (mean - y.engMean) * y.kEng
		y.engMS += (ms - y.engMS) * y.kEng
	}
}

// jet renders the jet noise layer for one sample.
func (y *Synth) jet() float32 {
	// The jet noise follows the cut through a short ramp against clicks.
	if d := y.gateTo - y.gate; d != 0 {
		y.gate += max(-gateStep, min(gateStep, d))
	}
	return y.jetBP.Process(y.noise2.Bipolar()) * y.jetGain * y.gate
}
