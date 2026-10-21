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
	// voicesPerCyl is the number of voices of each cylinder. A pulse can
	// last longer than one engine cycle at high RPM, so a cylinder fires
	// its voices in turn.
	voicesPerCyl = 2
	// maxRPM bounds the input RPM.
	maxRPM = 20000
	// harmonicHz is the cutoff of the low pass of the pulse, the
	// engine-sim low pass of the synthesiser input.
	harmonicHz = 1900
	// derivNormHz is the frequency at which the derivative of the
	// synthesiser input has the gain of the pulse.
	derivNormHz = 400
	// inputDCHz is the cutoff of the engine-sim low pass whose output the
	// pressure term of the synthesiser input subtracts.
	inputDCHz = 10
	// jitterRate, jitterSteps and jitterHz are the engine-sim input
	// jitter: at an input_jitter of 1, the delay of the input moves over
	// jitterSteps samples at jitterRate, through a low pass at jitterHz.
	jitterRate  = 44100
	jitterSteps = 9
	jitterHz    = 10000
	// jitterLen is the length of the jitter delay line, a power of two
	// above the longest jitter delay at 96 kHz.
	jitterLen = 32
	// jitterLag is the lag in samples between the jitter offsets of two
	// paths, and jitterHist the length of the offset history, a power of
	// two above the lag of the last path.
	jitterLag  = 7
	jitterHist = 32
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
	// sqrt3 is the ratio of the half width of a uniform spread to its
	// standard deviation.
	sqrt3 = 1.7320508075688772
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
// pulses that start on the whole sample at or after their position.
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

// voice is the pulse of one firing. pos is the position in table steps,
// negative before the pulse starts, and from the sample of the chunk from
// which the voice runs. tab is the pulse table of the firing.
type voice struct {
	tab        []float32
	pos, gain  float32
	bank, from int
	active     bool
}

// Synth renders one engine. It allocates nothing after New.
type Synth struct {
	rate float64

	noise  dsp.Noise
	idle   idle
	crank  crank
	voices []voice
	alt    []uint8
	// act lists the active voices, so that the pulse loop visits only
	// them. bankBuf holds the pulse sum of each bank over the chunk.
	act          []int
	bankBuf      [MaxBanks][chunk]float32
	delay        []float32
	cylGain      []float32
	cylDev       []float32
	stepPerCycle float32
	maxInc       float32
	invRate120   float32

	// The pulse table from engine-sim (measured). tab is the row or the
	// blend of two rows that the firings of the chunk take.
	pt  *pulseTable
	tab []float32
	// measVar is the share of the measured variation of the table, and
	// spreadG the half width of the uniform firing gain for the chunk.
	measVar, spreadG float32

	banks    int
	cross    [MaxBanks][MaxBanks]float32
	paths    []path
	formants formants
	popBank  popBank
	pops     pops
	out      output

	// Profile constants.
	cycleVar, jitter            float32
	redline, camRPM             float32
	camGain                     float32
	voiceGain, overlap          float32
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
	shape                       float32
	rpm, throttle, load, master float32
	overrun, camMix             float32
	camHigh                     bool
	inc, step, amp              float32

	// The engine-sim synthesiser input of each path. hLP is the fourth-
	// order low pass of the pulse, and hPrev its last output, for the
	// derivative. inDC is the low pass that the pressure term subtracts.
	// amLP is the fourth-order low pass of the AM noise. kD is the gain of
	// the derivative per sample step, and kNoise and kFlat the gains of
	// the noise and of the pulse in the AM term.
	hLP               [MaxBanks][2]dsp.Biquad
	hPrev             [MaxBanks]float32
	inDC              [MaxBanks]dsp.OnePole
	amLP              [2]dsp.Biquad
	kD, kNoise, kFlat float32

	// The engine-sim input jitter of each path, after hLP. jitScale is
	// the range of the random read offset in samples, and jitMax the
	// longest delay. With no jitter, the stage does not run.
	inJit            [MaxBanks]inputJitter
	jitNoise         dsp.Noise
	jitScale, jitMax float32
	// jitLP is the low pass of the offset, and jitHist the offsets of the
	// last jitterHist samples, which the paths read at their lags.
	jitLP   dsp.Biquad
	jitHist [jitterHist]float32
	jitW    int

	// Engine level for the pops. engS and engE sum the levelled engine and
	// its square over the chunk, which is where the pops join it. engMean
	// and engMS follow their means over the chunks with no pop with the
	// coefficient kEng, and hold while a pop sounds. The level is the RMS
	// level with no DC, which the output stage removes.
	engS, engE, engMean, engMS, kEng float32

	// Rev limiter state. cut is the samples left of the cut, and cutLen is
	// the length of the cut in samples. cutting is
	// true for a chunk inside a cut. bounce is true while the snapshot
	// holds the engine at the limiter, and climb is true while the
	// internal RPM lrpm climbs back to the limiter RPM after a cut of a
	// bounce. dip is the pitch offset below the smoothed RPM. limRot
	// counts the cuts. bangAt is the time into an ignition cut in samples
	// at which the unburnt fuel bangs, or -1. gear is the last gear. pump
	// is the level of the pumping in a cut, notch is the phase of the notch
	// in cycles, and bark is the extra firing level after a cut. fade is
	// the firing level after the final gear ends a cut. It starts at the
	// pump level and rises by the factor fadeRate per chunk to 1. These
	// levels set env, the gain of the levelled engine, which ramps by
	// envStep per sample over each chunk, so that the leveller does not
	// undo them.
	cut, cutLen, limRot int
	bangAt              int
	cutting, limEvent   bool
	bounce, climb       bool
	gear                uint8
	lrpm, dip           float32
	pump, notch, bark   float32
	fade, fadeRate      float32
	env, envStep        float32

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
		invRate120: float32(1 / (120 * fs)),
		banks:      s.Banks(),
		cycleVar:   float32(s.Pulse.CycleVariation),
		measVar:    float32(s.Pulse.MeasuredVariation),
		jitter:     float32(s.Pulse.Jitter),
		redline:    float32(s.RedlineRPM),
		camRPM:     float32(s.CamSwitchRPM),
		camGain:    float32(s.Pulse.CamGain),
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
		env:        -1,
		kRPM:       dsp.TimeCoef(0.012, chunkHz),
		kCtl:       dsp.TimeCoef(0.025, chunkHz),
		kMaster:    dsp.TimeCoef(0.02, chunkHz),
		kCam:       dsp.TimeCoef(0.015, chunkHz),
		kLim:       dsp.TimeCoef(0.002, chunkHz),
		kHand:      1 - dsp.TimeCoef(handover, chunkHz),
		kEng:       dsp.TimeCoef(engLevelSec, chunkHz),
		jit:        dsp.OnePole{A: dsp.OnePoleCoef(2, chunkHz)},
		idleRPM:    float32(s.IdleRPM),
		ovLevel:    float32(math.Pow(10, s.Overrun.LevelDB/20)),
		ovIdle:     float32(math.Pow(10, s.Overrun.IdleLevelDB/20)),
		partLoad:   float32(math.Pow(10, s.Pulse.PartLoadDB/20)),
		pops:       newPops(s, fs, float64(controlBlock)/fs),
		out:        newOutput(s.Voice.LevellerDB, s.Voice.OutputDB, fs),
	}
	y.initSynthInput(s, fs)
	// Validate has loaded the table.
	y.pt, _ = loadPulse(s.Pulse.Table)
	y.tab = y.pt.v[0]

	y.idle = newIdle(s, seed, chunkHz)
	y.initCylinders(s, fs)
	y.initPaths(s, fs)
	y.pops.calibrateCached(s, fs)
	return y, nil
}

func (y *Synth) initCylinders(s *Spec, fs float64) {
	n := len(s.Cylinders)
	y.voices = make([]voice, voicesPerCyl*n)
	y.alt = make([]uint8, n)
	y.act = make([]int, 0, voicesPerCyl*n)
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
		for v := range voicesPerCyl {
			y.voices[voicesPerCyl*i+v].bank = c.Bank
		}
		y.delay[i] = float32(c.PrimaryM / s.Exhaust.SpeedOfSound * fs)
	}
	y.crank = newCrank(offsets)
	ev := y.crank.events
	gap := 1 - ev[n-1].offset + ev[0].offset
	for i := 1; i < n; i++ {
		gap = min(gap, ev[i].offset-ev[i-1].offset)
	}
	y.maxInc = 0.9 * gap
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
				y.cross[p][b] = 1
			case p == b:
				y.cross[p][b] = float32(1 - k)
			default:
				y.cross[p][b] = float32(k / float64(nb-1))
			}
		}
	}
	y.paths = make([]path, nb)
	for i, ps := range s.Paths {
		y.paths[i] = newPath(ps, s, fs)
	}
	y.formants = newFormants(s.Formants, s.Exhaust.FormantGainDB, fs)
	for _, f := range s.Formants {
		y.out.noEngDC = y.out.noEngDC || f.Kind == "highpass"
	}
	y.popBank = popBank{f: newFormants(s.PopFormants, s.Exhaust.PopFormantGainDB, fs)}
}

// initSynthInput sets the stages from engine-sim that make the input of
// each path from its pulse sum x, as Synthesizer::renderAudio makes the
// input of its convolution: a fourth-order Butterworth low pass at
// harmonicHz, the input jitter, then hf_gain times the derivative plus
// 1-hf_gain times x less its 10 Hz low pass times the AM term am*r+1-am,
// where r is uniform noise through a fourth-order Butterworth low pass at
// am_cutoff_hz. The derivative is per second, as in engine-sim, so its
// gain does not depend on the sample rate. The input scales by
// 1/(1-hf_gain+hf_gain*2*pi*derivNormHz), which gives the derivative of a
// sine at derivNormHz the gain of the pulse, so that the input keeps about
// the scale of the pulse. The leveller removes the scale.
func (y *Synth) initSynthInput(s *Spec, fs float64) {
	hf := s.Noise.HFGain
	norm := 1 / (1 - hf + hf*2*math.Pi*derivNormHz)
	y.kD = float32(hf * fs * norm)
	y.kNoise = float32((1 - hf) * s.Noise.AM * norm)
	y.kFlat = float32((1 - hf) * (1 - s.Noise.AM) * norm)
	dt := 1 / fs
	rc := 1 / (2 * math.Pi * inputDCHz)
	for p := range y.banks {
		y.hLP[p][0].SetLowPass(harmonicHz, 0.5412, fs)
		y.hLP[p][1].SetLowPass(harmonicHz, 1.3066, fs)
		y.inDC[p].A = float32(dt / (rc + dt))
	}
	y.jitLP.SetLowPass(jitterHz, 0.7071, fs)
	y.amLP[0].SetLowPass(s.Noise.AMCutoffHz, 0.5412, fs)
	y.amLP[1].SetLowPass(s.Noise.AMCutoffHz, 1.3066, fs)
	steps := min(jitterSteps*fs/jitterRate, jitterLen-2)
	y.jitScale = float32(steps * s.Noise.InputJitter)
	y.jitMax = float32(steps)
	y.jitNoise = dsp.NewNoise(uint32(s.Seed&math.MaxUint16) + 0x3c6e)
}

// inputJitter is the engine-sim input jitter of one path (JitterFilter):
// the input passes a short delay line, and the read point moves at random,
// from uniform noise through a low pass, with linear interpolation between
// two samples. engine-sim uses a fourth-order Butterworth low pass. One
// second-order section at the same cutoff costs half as much, and the 6 to
// 16 kHz noise of the tuned profiles stays within 0.5 dB. The engine draws
// one low-passed offset per sample, and path p reads it p*jitterLag samples
// late: the offset at 10 kHz has no correlation over that lag, so the
// paths jitter independently, as the engine-sim channels do, for one noise
// draw and one low pass.
type inputJitter struct {
	hist [jitterLen]float32
	w    int
}

// process writes x and returns the input at the random delay. u is the
// low-passed random offset from 0 to the jitter range in samples, and most
// the longest delay.
func (j *inputJitter) process(x, u, most float32) float32 {
	j.w = (j.w + 1) & (jitterLen - 1)
	j.hist[j.w] = x
	// The low pass can overshoot the range of u by a few per cent. The
	// masks keep every read inside the delay line, so no clamp is needed:
	// a delay slightly below 0 reads the newest two samples.
	d := most - u
	k := int(d)
	f := d - float32(k)
	a := j.hist[(j.w-k)&(jitterLen-1)]
	b := j.hist[(j.w-k-1)&(jitterLen-1)]
	return a + (b-a)*f
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
	if !c.Running && y.master < snapLevel {
		// The decay stops on a subnormal value, which would slow every
		// stage after the voices.
		y.master = 0
	}
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
	// The limiter level ramps over the chunk. It starts at its first value.
	g := (1 + y.bark) * y.fade
	if y.cutting {
		g = y.pump
	}
	if y.env < 0 {
		y.env = g
	}
	y.envStep = (g - y.env) / float32(n)

	heat := b2f(y.throttle > 0.2) * y.load * min(rr, 1)
	y.pops.control(&popInput{
		rpm: y.rpm, throttle: thr, overrun: over && y.master > 0.5, cut: limiting, off: c.NoPops,
		heat: heat, character: c.Character, level: float32(math.Sqrt(math.Max(0, float64(y.engMS-y.engMean*y.engMean)))),
		n: n, paths: len(y.paths),
	})
	y.controlPulse()
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

// controlPulse sets the pulse table and its step for one chunk. Between two
// rows of the table, the pulse is their blend at the same share of their
// lengths, weighted by the distance in pitch RPM in steps of 1/blendSteps,
// and its length is the blend of their lengths. When the cam switch RPM
// lies between the two rows, the cam crossfade sets the weight, so that
// the pulse changes at the cam switch, as the engine-sim cam profile
// does. A firing keeps the table of its chunk, and the step of every
// sounding pulse follows the length.
func (y *Synth) controlPulse() {
	t := y.pt
	a, b, w := t.bracket(float64(y.PitchRPM()))
	if y.camRPM > 0 && a != b && t.rpm[a] < float64(y.camRPM) && t.rpm[b] >= float64(y.camRPM) {
		w = float64(y.camMix)
	}
	l := t.lenSec[a] + (t.lenSec[b]-t.lenSec[a])*w
	y.tab = t.at(a, b, w)
	if y.measVar > 0 {
		// A uniform spread from -h to h has a standard deviation of
		// h/sqrt(3).
		y.spreadG = y.measVar * float32(sqrt3*(t.gainSD[a]+(t.gainSD[b]-t.gainSD[a])*w))
	}
	y.step = float32(float64(len(y.tab)-1) / (l * y.rate))
	y.stepPerCycle = 0
	if y.inc > 0 {
		y.stepPerCycle = y.step / y.inc
	}
}

// fire starts the pulse of one cylinder at sample at of the chunk. In a
// limiter cut, the pulse is a pumping pulse, with no combustion variation
// in gain or delay. The limiter levels act on the levelled engine, through
// env.
func (y *Synth) fire(cyl, at int) {
	g, late := float32(1), float32(0)
	if y.cutting {
		if y.bangAt >= 0 && y.cutLen-y.cut >= y.bangAt {
			y.pops.bang(len(y.paths))
			y.bangAt = -1
		}
	} else {
		g, late = y.idle.firing()
		// A spread above 1 clips at zero, so that no firing has a negative
		// pressure.
		g *= max(0, 1+(y.cycleVar+y.spreadG)*y.noise.Bipolar())
	}
	// The cylinder fires its voices in turn, so that a pulse that lasts
	// longer than one engine cycle rings on under the next.
	k := voicesPerCyl*cyl + int(y.alt[cyl])
	y.alt[cyl] = (y.alt[cyl] + 1) % voicesPerCyl
	v := &y.voices[k]
	listed := v.active
	if listed {
		// The earlier pulse of the voice runs up to the new firing.
		y.renderVoice(v, at)
	}
	v.gain = y.amp * (y.cylGain[cyl] + y.idle.imbalance*y.cylDev[cyl]) * g
	v.tab = y.tab
	v.pos = (y.crank.past-late)*y.stepPerCycle - y.delay[cyl]*y.step
	if wholeSampleStart && y.step > 0 {
		v.pos = -float32(math.Ceil(float64(-v.pos/y.step))) * y.step
	}
	v.from = at
	v.active = true
	if !listed {
		y.act = append(y.act, k)
	}
}

// renderVoices adds every active voice to the bank sums of the first n
// samples of the chunk, and drops the voices that end from the list.
//
// The pulse is the table from engine-sim (measured): the runner pressure
// of one firing, which rises smoothly from zero and falls into a negative
// lobe. A pulse starts at its fractional position: its first sample is
// the table at v.pos steps after the start, by linear interpolation. The
// table starts and ends at zero, so the start and the end need no
// band-limited corner. Each voice runs over the chunk in one loop, from
// the sample at which it fired, which gives the same sum as one sample at
// a time.
func (y *Synth) renderVoices(n int) {
	act := y.act
	for k := 0; k < len(act); {
		v := &y.voices[act[k]]
		y.renderVoice(v, n)
		v.from = 0
		if !v.active {
			// The voice ended: the last active voice takes its place.
			act[k] = act[len(act)-1]
			act = act[:len(act)-1]
			continue
		}
		k++
	}
	y.act = act
}

// renderVoice adds voice v to its bank sum from sample v.from to the
// sample before to.
func (y *Synth) renderVoice(v *voice, to int) {
	buf := y.bankBuf[v.bank][:to]
	step, tab := y.step, v.tab
	last := len(tab) - 1
	pos, g := v.pos, v.gain
	i := v.from
	for ; i < len(buf) && pos < 0; i++ {
		pos += step
	}
	for ; i < len(buf); i++ {
		k := int(pos)
		if k >= last {
			v.active = false
			break
		}
		t := tab[k : k+2]
		a := t[0]
		buf[i] += (a + (t[1]-a)*(pos-float32(k))) * g
		pos += step
	}
	v.pos, v.from = pos, to
}

// process renders one chunk.
func (y *Synth) process(dst []float32) {
	np := len(y.paths)
	n := len(dst)
	for b := range np {
		clear(y.bankBuf[b][:n])
	}
	for i := range n {
		if cyl := y.crank.step(y.inc); cyl >= 0 {
			y.fire(cyl, i)
		}
	}
	y.renderVoices(n)
	for i := range dst {
		var bank [MaxBanks]float32
		for b := range np {
			v := y.bankBuf[b][i]
			bank[b] = v + y.overlap*v*v
		}

		am := y.kNoise*y.amLP[1].Process(y.amLP[0].Process(y.noise.Bipolar())) + y.kFlat
		pop := y.pops.tick()
		if y.jitScale > 0 {
			y.jitW = (y.jitW + 1) & (jitterHist - 1)
			y.jitHist[y.jitW] = y.jitLP.Process(y.jitNoise.Unipolar() * y.jitScale)
		}

		var sum, pre float32
		run := false
		for p := range np {
			var x float32
			for b := range np {
				x += y.cross[p][b] * bank[b]
			}
			// From engine-sim: the low pass and the jitter of the
			// synthesiser input, and the derivative plus the noise AM of
			// the pressure with no DC.
			x = y.hLP[p][1].Process(y.hLP[p][0].Process(x))
			if y.jitScale > 0 {
				u := y.jitHist[(y.jitW-p*jitterLag)&(jitterHist-1)]
				x = y.inJit[p].process(x, u, y.jitMax)
			}
			d := x - y.hPrev[p]
			y.hPrev[p] = x
			f := x - y.inDC[p].Process(x)
			sum += y.paths[p].process(y.kD*d + f*am)
			if y.pops.live[p] || y.paths[p].popLeft > 0 {
				pre += y.paths[p].popPre(y.pops.jump[p], y.pops.live[p])
				run = true
			}
		}
		// One pop bank on the sum of the pop stages of the paths.
		pop += y.popBank.process(pre, run)
		// From engine-sim: the impulse response, as a formant bank on the
		// sum of the paths.
		sum = y.formants.process(sum)
		y.env += y.envStep
		e := y.out.level(sum) * y.env
		y.engS += e
		y.engE += e * e
		dst[i] = y.out.process(e + pop)
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
