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
	// tableSize is the length of the pulse table.
	tableSize = 512
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
	// airSwell is the decay time constant in seconds of the air noise
	// swell at a lift-off, and airSteady is the steady air level after it,
	// relative to the swell peak.
	airSwell  = 0.12
	airSteady = 0.25
	// airGain makes up the level that the air noise low passes take out.
	airGain = 10
	// handover is the time constant in seconds over which the pitch
	// returns from the internal limiter RPM to the smoothed RPM.
	handover = 0.017
)

// Control is the engine state for one block.
type Control struct {
	RPM, Throttle, Load float32
	Running, Overrun    bool
	// LimiterEvents is the number of rev limiter events since the last
	// block.
	LimiterEvents int
	// Gear is the gear of the snapshot. A change ends a limiter bounce.
	Gear uint8
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
	tier1 bool
	rate  float64

	noise, noise2 dsp.Noise
	idle          idle
	crank         crank
	voices        []voice
	delay         []float32
	cylGain       []float32
	cylDev        []float32
	tabLow        []float32
	tabHigh       []float32
	stepPerCycle  float32
	maxInc        float32
	invRate120    float32

	banks int
	mix   [MaxBanks][MaxBanks]float32
	paths []path
	pops  pops
	out   output

	// Profile constants.
	idleFloor, cycleVar, jitter  float32
	redline, camRPM              float32
	camGain, camAttack, attack   float32
	decay                        float32
	amCfg, airCfg, jetCfg        float32
	intakeCfg, intakeCamGain     float32
	intakeHz, intakeCamHz        float32
	fanOrder, fanCfg, tickCfg    float32
	fanInc                       float32
	steepCfg, voiceGain          float32
	overlap                      float32
	limKind                      string
	limRPM, limFall, limRise     float32
	limCut                       int
	pumpLvl, notchLvl, barkLvl   float32
	notchStep, kBark             float32
	kRPM, kCtl, kMaster, kCam    float32
	kLim, kHand                  float32
	jit                          dsp.OnePole
	amLP, airLP1, airLP2         dsp.OnePole
	swell, swellDecay            float32
	wasOver                      bool
	idleRPM, ovLevel, ovIdle     float32
	partLoad                     float32
	idleGain, knee               float32
	rpmG, rpmStep                float32
	shape, lift                  float32
	intakeBP, jetBP, tickHP      dsp.Biquad
	intakeSetMix                 float32
	tickDec                      float32
	fanPhase, tickEnv            float32
	rpm, throttle, load, master  float32
	overrun, camMix              float32
	camHigh                      bool
	inc, step, amp, amDepth, air float32
	jetGain, intakeGain, fanGain float32
	tickGain, ka, kb, liveNorm   float32

	// Rev limiter state. cut is the samples left of the cut. cutting is
	// true for a chunk inside a cut. bounce is true while the snapshot
	// holds the engine at the limiter, and climb is true while the
	// internal RPM lrpm climbs back to the limiter RPM after a cut of a
	// bounce. dip is the pitch offset below the smoothed RPM. limRot
	// counts the cuts. bangAt is the time into an ignition cut in samples
	// at which the unburnt fuel bangs, or -1. gate is the jet noise gain of
	// the cut, and gateTo is its target. gear is the last gear. pump is
	// the gain of a pumping pulse in a cut, notch is the phase of the notch
	// in cycles, and bark is the extra firing gain after a cut.
	cut, limRot             int
	bangAt                  int
	cutting, limEvent       bool
	bounce, climb           bool
	gear                    uint8
	lrpm, dip, gate, gateTo float32
	pump, notch, bark       float32
}

// New returns a synth for the spec at the sample rate. tier1 selects the
// tier 1 model. Otherwise the synth runs tier 0.
func New(s *Spec, rate int, tier1 bool) (*Synth, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if rate < 8000 {
		return nil, errors.New("sample rate is below 8000 Hz")
	}
	fs := float64(rate)
	chunkHz := fs / chunk
	seed := uint32(s.Seed & math.MaxUint16)
	y := &Synth{
		tier1:      tier1,
		rate:       fs,
		noise:      dsp.NewNoise(seed),
		noise2:     dsp.NewNoise(seed + 0x51ed),
		invRate120: float32(1 / (120 * fs)),
		banks:      s.Banks(),
		idleFloor:  float32(s.Pulse.IdleFloor),
		cycleVar:   float32(s.Pulse.CycleVariation),
		jitter:     float32(s.Pulse.Jitter),
		redline:    float32(s.RedlineRPM),
		camRPM:     float32(s.CamSwitchRPM),
		camGain:    float32(s.Pulse.CamGain),
		attack:     float32(s.Pulse.Attack),
		camAttack:  float32(s.Pulse.CamAttack),
		decay:      float32(s.Pulse.Decay),
		amCfg:      float32(s.Noise.AM),
		airCfg:     float32(airGain * s.Noise.Air),
		jetCfg:     float32(s.Noise.Jet),
		intakeCfg:  float32(s.Noise.Intake),
		intakeHz:   float32(s.Noise.IntakeHz),
		fanOrder:   float32(2 * s.Mechanical.FanRatio * s.Mechanical.FanBlades),
		fanCfg:     float32(s.Mechanical.FanGain),
		tickCfg:    float32(s.Mechanical.TickGain),
		steepCfg:   float32(s.Exhaust.Steepening),
		overlap:    float32(s.Exhaust.Overlap),
		voiceGain:  float32(s.Voice.Gain),
		limKind:    s.LimiterKind,
		limRPM:     float32(s.LimiterRPM),
		limFall:    float32(s.LimiterFallRPMS * s.LimiterLoadScale / fs),
		limRise:    float32(s.LimiterRiseRPMS * s.LimiterLoadScale / fs),
		limCut:     int(s.LimiterMS * 1e-3 * fs),
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
		jit:        dsp.OnePole{A: dsp.OnePoleCoef(2, chunkHz)},
		amLP:       dsp.OnePole{A: dsp.OnePoleCoef(2000, fs)},
		airLP1:     dsp.OnePole{A: dsp.OnePoleCoef(s.Noise.AirCutoffHz, fs)},
		airLP2:     dsp.OnePole{A: dsp.OnePoleCoef(s.Noise.AirCutoffHz, fs)},
		swellDecay: 1 - dsp.TimeCoef(airSwell, chunkHz),
		idleRPM:    float32(s.IdleRPM),
		ovLevel:    float32(math.Pow(10, s.Overrun.LevelDB/20)),
		ovIdle:     float32(math.Pow(10, s.Overrun.IdleLevelDB/20)),
		partLoad:   float32(math.Pow(10, s.Pulse.PartLoadDB/20)),
		rpmG:       -1,
		tickDec:    dsp.DecayCoef(40, 0.004, fs),
		pops:       newPops(s, fs, chunk/fs, tier1),
	}
	outDB := s.Voice.OutputDB
	if tier1 {
		outDB += s.Voice.Tier1DB
	}
	y.out = newOutput(outDB, fs)
	y.knee = float32(max(s.Voice.IdleGainRPM, s.IdleRPM))
	y.idleGain = float32(s.Voice.IdleGainDB)
	if tier1 {
		y.idleGain += float32(s.Voice.Tier1IdleGainDB)
	}
	if y.camAttack <= 0 {
		y.camAttack = y.attack
	}
	y.intakeCamGain = float32(s.Noise.IntakeCamGain)
	y.intakeCamHz = float32(s.Noise.IntakeCamHz)
	if y.intakeCamHz <= 0 {
		y.intakeCamHz = y.intakeHz
	}
	if y.intakeHz > 0 {
		y.intakeBP.SetBandPass(float64(y.intakeHz), 1.5, fs)
	}
	jetHz := s.Noise.JetHz
	if jetHz <= 0 {
		jetHz = 1500
	}
	y.jetBP.SetBandPass(jetHz, 0.7, fs)
	y.tickHP.SetHighPass(s.Mechanical.TickHz, 0.9, fs)

	y.idle = newIdle(s, seed, chunkHz)
	y.initCylinders(s, fs)
	y.initPaths(s, fs)
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
	y.stepPerCycle = float32(tableSize / width)
	y.tabLow = pulseTable(s.Pulse.Attack, s.Pulse.Decay)
	y.tabHigh = y.tabLow
	if s.CamSwitchRPM > 0 && s.Pulse.CamAttack > 0 {
		y.tabHigh = pulseTable(s.Pulse.CamAttack, s.Pulse.Decay)
	}
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

// pulseTable returns the envelope (1-exp(-a x))exp(-b x) over x from 0 to
// 1, with a peak of 1 and a short fade to zero at the end.
func pulseTable(a, b float64) []float32 {
	t := make([]float32, tableSize+1)
	var peak float64
	v := make([]float64, tableSize)
	for i := range v {
		x := float64(i) / tableSize
		v[i] = (1 - math.Exp(-a*x)) * math.Exp(-b*x)
		if x > 0.9 {
			v[i] *= (1 - x) / 0.1
		}
		peak = math.Max(peak, v[i])
	}
	for i := range v {
		t[i] = float32(v[i] / peak)
	}
	return t
}

// envelopePeak returns the peak of (1-exp(-a x))exp(-b x) for x >= 0.
func envelopePeak(a, b float64) float64 {
	x := math.Log((a+b)/b) / a
	return (1 - math.Exp(-a*x)) * math.Exp(-b*x)
}

// PitchRPM returns the RPM that the firing follows. It differs from the
// smoothed input RPM in a limiter bounce and in the handover after it.
func (y *Synth) PitchRPM() float32 {
	return max(0, y.rpm-y.dip)
}

// Render fills dst. It allocates nothing.
func (y *Synth) Render(dst []float32, c Control) {
	if c.LimiterEvents > 0 {
		y.limEvent = true
	}
	for len(dst) > 0 {
		n := min(len(dst), chunk)
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
	thr := clampf(c.Throttle, 0, 1)
	load := clampf(c.Load, 0, 1)
	y.rpm += (rpm - y.rpm) * y.kRPM
	y.throttle += (thr - y.throttle) * y.kCtl
	y.load += (load - y.load) * y.kCtl
	y.master += (b2f(c.Running) - y.master) * y.kMaster
	limiting := y.limit(rpm, thr, c.Gear, n)
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
	// The fan follows the smoothed RPM, not the pitch, so that it holds its
	// pitch in a limiter bounce.
	y.fanInc = y.rpm * speed * y.fanOrder
	y.step = y.inc * y.stepPerCycle

	ld := float64(y.load)
	rr := min(y.rpm/y.redline, 1.2)
	// The pulse gain follows the load curve. On overrun, it moves to the
	// overrun level. lift is the overrun gain over the idle floor. The pops
	// follow it, so that they keep their balance with the pulses.
	pos := min(1, max(0, (y.rpm-y.idleRPM)/(y.redline-y.idleRPM)))
	lvl := y.ovIdle + (y.ovLevel-y.ovIdle)*pos
	shape := loadLevel(ld, lvl, y.partLoad)
	y.shape = shape + (lvl-shape)*y.overrun
	y.lift = 1 + (lvl/max(y.idleFloor, 0.05)-1)*y.overrun
	y.amp = y.master * y.voiceGain * y.shape * (0.5 + 0.5*rr) *
		(1 + y.camGain*y.camMix) * (1 + 2*y.jitter*j) * y.idle.rock
	gate := float32(math.Pow(math.Max(ld, 0.02), 0.7))
	y.amDepth = y.amCfg * gate
	// The air noise swells at a lift-off and falls to a low steady level.
	if over && !y.wasOver {
		y.swell = 1
	}
	y.wasOver = over
	y.swell *= y.swellDecay
	y.air = y.airCfg * y.overrun * y.master * (0.3 + rr) * (airSteady + (1-airSteady)*y.swell)

	// The RPM gain ramps over the chunk. It starts at its first value.
	g := rpmGain(y.rpm, y.knee, y.redline, y.idleGain)
	if y.rpmG < 0 {
		y.rpmG = g
	}
	y.rpmStep = (g - y.rpmG) / float32(n)

	heat := b2f(y.throttle > 0.2) * y.load * min(rr, 1)
	y.pops.control(y.rpm, y.throttle, over && y.master > 0.5, limiting, heat, &y.noise)

	if y.tier1 {
		// The exhaust flow layers follow the pulse gain, so that the level
		// order over load holds in tier 1 as in tier 0.
		y.controlTier1(y.shape, rr)
	}
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
func (y *Synth) limit(rpm, thr float32, gear uint8, n int) bool {
	was := y.cutting
	y.bark *= y.kBark
	// The smoothed RPM must be near the limiter too, so that a bounce
	// starts from the limiter RPM.
	at := y.limRPM > 0 && rpm >= y.limRPM*0.998 && y.rpm >= y.limRPM*0.99 && thr > 0.1 && gear == y.gear
	y.gear = gear
	if y.bounce && !at {
		y.bounce, y.climb, y.cut = false, false, 0
	}
	if y.cut <= 0 && !y.climb && (at || y.limEvent) && y.limCut > 0 {
		y.cut = y.limCut
		y.bounce = at
		y.lrpm = y.rpm - y.dip
		y.notch = 0
		y.limRot++
		y.bangAt = -1
		// An ignition cut bangs with a chance of the pop density, at a
		// random time in the cut.
		if y.limKind == "ignition" && y.noise.Unipolar() < y.pops.density {
			y.bangAt = int((0.2 + 0.8*y.noise.Unipolar()) * float32(y.limCut))
		}
	}
	y.limEvent = false
	y.cutting = y.cut > 0
	if was && !y.cutting {
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
		return false
	}
	y.dip = y.rpm - y.lrpm
	return true
}

func (y *Synth) controlTier1(flow, rr float32) {
	a := float64(y.attack + (y.camAttack-y.attack)*y.camMix)
	b := float64(y.decay)
	dx := float64(y.step) / tableSize
	y.ka = float32(math.Exp(-a * dx))
	y.kb = float32(math.Exp(-b * dx))
	y.liveNorm = float32(1 / envelopePeak(a, b))
	for i := range y.paths {
		y.paths[i].steep = y.steepCfg * flow
	}
	jet := float32(math.Pow(math.Max(float64(flow), 0.02), 0.7)) * rr
	y.jetGain = y.master * y.jetCfg * jet * jet
	y.intakeGain = y.master * y.intakeCfg * (0.2 + 0.8*y.throttle) *
		(1 + y.intakeCamGain*y.camMix)
	if y.intakeHz > 0 && y.intakeCamHz != y.intakeHz {
		if d := y.camMix - y.intakeSetMix; d > 0.02 || d < -0.02 {
			y.intakeSetMix = y.camMix
			f := y.intakeHz + (y.intakeCamHz-y.intakeHz)*y.camMix
			y.intakeBP.SetBandPass(float64(f), 1.5, y.rate)
		}
	}
	// The fan and the valve tick follow the pulse level, so that the
	// exhaust stays on top on overrun.
	y.fanGain = y.master * y.fanCfg * (0.3 + 0.7*rr) * y.shape
	y.tickGain = y.master * y.tickCfg * (0.4 + 0.6*rr) * y.shape
}

// fire starts the pulse of one cylinder. In a limiter cut, the pulse is a
// pumping pulse, with no combustion variation in gain or delay.
func (y *Synth) fire(cyl int) {
	var g, late float32
	if y.cutting {
		g = y.pump
		if y.bangAt >= 0 && y.limCut-y.cut >= y.bangAt {
			y.pops.bang(&y.noise, len(y.paths))
			y.bangAt = -1
		}
	} else {
		g, late = y.idle.firing()
		g *= (1 + y.bark) * (1 + y.cycleVar*y.noise.Bipolar())
	}
	v := &y.voices[cyl]
	v.gain = y.amp * (y.cylGain[cyl] + y.idle.imbalance*y.cylDev[cyl]) * g
	v.pos = (y.crank.past-late)*y.stepPerCycle - y.delay[cyl]*y.step
	v.active = true
	v.started = false
	y.tickEnv = y.tickGain
}

// pulses runs every active voice for one sample and sums the voices per
// bank.
func (y *Synth) pulses(bank *[MaxBanks]float32) {
	step := y.step
	lo, hi := y.tabLow, y.tabHigh
	cam := y.camMix
	for k := range y.voices {
		v := &y.voices[k]
		if !v.active {
			continue
		}
		if v.pos < 0 {
			v.pos += step
			continue
		}
		i := int(v.pos)
		if i >= tableSize {
			v.active = false
			continue
		}
		var e float32
		if y.tier1 {
			if !v.started {
				v.started = true
				v.e1, v.e2 = 1, 1
			}
			e = (1 - v.e1) * v.e2 * y.liveNorm
			v.e1 *= y.ka
			v.e2 *= y.kb
		} else {
			f := v.pos - float32(i)
			a := lo[i]
			e = a + f*(lo[i+1]-a)
			if cam > 0.001 {
				b := hi[i]
				e += cam * (b + f*(hi[i+1]-b) - e)
			}
		}
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

		nz := y.amLP.Process(y.noise.Bipolar()) * noiseScale
		am := 1 + y.amDepth*nz
		var airN float32
		if y.air != 0 {
			airN = y.air * y.airLP2.Process(y.airLP1.Process(nz))
		}
		jump, crack := y.pops.tick(&y.noise, np)

		var sum float32
		for p := range np {
			var x float32
			for b := range np {
				x += y.mix[p][b] * bank[b]
			}
			x = x*am + airN
			if p == y.pops.target {
				x += jump * y.lift
			}
			if y.tier1 {
				sum += y.paths[p].process1(x)
			} else {
				sum += y.paths[p].process0(x)
			}
		}
		sum += crack * y.lift
		if y.tier1 {
			sum += y.layers(&bank, np)
		}
		y.rpmG += y.rpmStep
		dst[i] = y.out.process(sum * y.rpmG)
	}
}

// layers renders the tier 1 intake, jet and mechanical layers.
func (y *Synth) layers(bank *[MaxBanks]float32, np int) float32 {
	var pulse float32
	for b := range np {
		pulse += bank[b]
	}
	n2 := y.noise2.Bipolar()
	// The jet noise follows the cut through a short ramp against clicks.
	if d := y.gateTo - y.gate; d != 0 {
		y.gate += max(-gateStep, min(gateStep, d))
	}
	out := y.jetBP.Process(n2) * y.jetGain * y.gate
	if y.intakeGain > 0 {
		out += y.intakeBP.Process(n2) * pulse * y.intakeGain
	}
	if y.fanGain > 0 {
		y.fanPhase += y.fanInc
		if y.fanPhase >= 1 {
			y.fanPhase--
		}
		out += dsp.Sine(y.fanPhase) * y.fanGain
	}
	if y.tickEnv > 1e-5 {
		out += y.tickHP.Process(n2) * y.tickEnv
		y.tickEnv *= y.tickDec
	}
	return out
}
