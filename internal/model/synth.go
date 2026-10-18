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
)

// Control is the engine state for one block.
type Control struct {
	RPM, Throttle, Load float32
	Running, Overrun    bool
	// LimiterEvents is the number of rev limiter events since the last
	// block.
	LimiterEvents int
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
	crank         crank
	voices        []voice
	delay         []float32
	cylGain       []float32
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
	steepCfg, voiceGain          float32
	overlap                      float32
	limKind                      string
	limSamples                   int
	kRPM, kCtl, kMaster, kCam    float32
	jit                          dsp.OnePole
	amLP                         dsp.OnePole
	idleRPM, ovLevel, ovIdle     float32
	partLoad                     float32
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
	cut, cutCount                int
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
		airCfg:     float32(s.Noise.Air),
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
		limSamples: int(s.LimiterMS * 1e-3 * fs),
		kRPM:       dsp.TimeCoef(0.012, chunkHz),
		kCtl:       dsp.TimeCoef(0.025, chunkHz),
		kMaster:    dsp.TimeCoef(0.02, chunkHz),
		kCam:       dsp.TimeCoef(0.015, chunkHz),
		jit:        dsp.OnePole{A: dsp.OnePoleCoef(2, chunkHz)},
		amLP:       dsp.OnePole{A: dsp.OnePoleCoef(2000, fs)},
		idleRPM:    float32(s.IdleRPM),
		ovLevel:    float32(math.Pow(10, s.Overrun.LevelDB/20)),
		ovIdle:     float32(math.Pow(10, s.Overrun.IdleLevelDB/20)),
		partLoad:   float32(math.Pow(10, s.Pulse.PartLoadDB/20)),
		tickDec:    dsp.DecayCoef(40, 0.004, fs),
		pops:       newPops(s, fs, chunk/fs, tier1),
	}
	outDB := s.Voice.OutputDB
	if tier1 {
		outDB += s.Voice.Tier1DB
	}
	y.out = newOutput(outDB, fs)
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

	y.initCylinders(s, fs)
	y.initPaths(s, fs)
	return y, nil
}

func (y *Synth) initCylinders(s *Spec, fs float64) {
	n := len(s.Cylinders)
	y.voices = make([]voice, n)
	y.delay = make([]float32, n)
	y.cylGain = make([]float32, n)
	offsets := make([]float32, n)
	setup := dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x7a11)
	for i, c := range s.Cylinders {
		deg := c.CrankDeg + s.Pulse.TimingVariationDeg*float64(setup.Bipolar())
		deg = math.Mod(deg+720, 720)
		y.cylGain[i] = 1 + float32(s.Pulse.GainVariation)*setup.Bipolar()
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

// Render fills dst. It allocates nothing.
func (y *Synth) Render(dst []float32, c Control) {
	if c.LimiterEvents > 0 {
		y.cut = y.limSamples
		y.cutCount = 0
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

// control runs once per chunk of n samples.
func (y *Synth) control(c *Control, n int) {
	rpm := clampf(c.RPM, 0, maxRPM)
	thr := clampf(c.Throttle, 0, 1)
	load := clampf(c.Load, 0, 1)
	y.rpm += (rpm - y.rpm) * y.kRPM
	y.throttle += (thr - y.throttle) * y.kCtl
	y.load += (load - y.load) * y.kCtl
	y.master += (b2f(c.Running) - y.master) * y.kMaster
	over := c.Overrun && thr < 0.05
	y.overrun += (b2f(over) - y.overrun) * y.kCtl

	if y.camRPM > 0 {
		if !y.camHigh && y.rpm > y.camRPM {
			y.camHigh = true
		} else if y.camHigh && y.rpm < y.camRPM-100 {
			y.camHigh = false
		}
		y.camMix += (b2f(y.camHigh) - y.camMix) * y.kCam
	}

	j := y.jit.Process(y.noise.Bipolar() * 1.7)
	inc := y.rpm * y.invRate120 * (1 + y.jitter*j)
	y.inc = min(inc, y.maxInc)
	y.step = y.inc * y.stepPerCycle

	ld := float64(y.load)
	rr := min(y.rpm/y.redline, 1.2)
	// The pulse gain follows the load curve. On overrun, it moves to the
	// overrun level. lift is the overrun gain over the idle floor. The air
	// noise and the pops follow it, so that they keep their balance with
	// the pulses.
	pos := min(1, max(0, (y.rpm-y.idleRPM)/(y.redline-y.idleRPM)))
	lvl := y.ovIdle + (y.ovLevel-y.ovIdle)*pos
	shape := loadLevel(ld, lvl, y.partLoad)
	y.shape = shape + (lvl-shape)*y.overrun
	y.lift = 1 + (lvl/max(y.idleFloor, 0.05)-1)*y.overrun
	y.amp = y.master * y.voiceGain * y.shape * (0.5 + 0.5*rr) *
		(1 + y.camGain*y.camMix) * (1 + 2*y.jitter*j)
	gate := float32(math.Pow(math.Max(ld, 0.02), 0.7))
	y.amDepth = y.amCfg * gate
	y.air = y.airCfg * y.overrun * y.master * (0.3 + rr) * y.lift

	if y.cut > 0 {
		y.cut -= n
	}
	heat := b2f(y.throttle > 0.2) * y.load * min(rr, 1)
	y.pops.control(y.rpm, y.throttle, over && y.master > 0.5, y.cut > 0, heat, &y.noise)

	if y.tier1 {
		// The exhaust flow layers follow the pulses to the overrun level.
		flow := y.load + (lvl-y.load)*y.overrun
		y.controlTier1(flow, rr)
	}
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

// fire starts the pulse of one cylinder.
func (y *Synth) fire(cyl int) {
	if y.cut > 0 {
		y.cutCount++
		switch y.limKind {
		case "injector":
			if y.cutCount&1 == 1 {
				return
			}
		case "ignition":
			y.pops.misfire(&y.noise, len(y.paths))
			return
		default:
			return
		}
	}
	v := &y.voices[cyl]
	v.gain = y.amp * y.cylGain[cyl] * (1 + y.cycleVar*y.noise.Bipolar())
	v.pos = y.crank.past*y.stepPerCycle - y.delay[cyl]*y.step
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
		airN := y.air * nz
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
		dst[i] = y.out.process(sum)
	}
}

// layers renders the tier 1 intake, jet and mechanical layers.
func (y *Synth) layers(bank *[MaxBanks]float32, np int) float32 {
	var pulse float32
	for b := range np {
		pulse += bank[b]
	}
	n2 := y.noise2.Bipolar()
	out := y.jetBP.Process(n2) * y.jetGain
	if y.intakeGain > 0 {
		out += y.intakeBP.Process(n2) * pulse * y.intakeGain
	}
	if y.fanGain > 0 {
		y.fanPhase += y.inc * y.fanOrder
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
