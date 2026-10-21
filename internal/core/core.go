package core

import (
	"errors"
	"fmt"
	"math"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/dsp"
	"github.com/oval-tutu/snarl/internal/esynth"
)

const (
	// maxRPM bounds the input RPM.
	maxRPM = 20000
	// synthRate is the rate of the engine-sim synthesiser.
	synthRate = 44100
	// block is the convolution block of the runtime. One control block
	// writes the simulation steps of one convolution block.
	block = esynth.DefaultRuntimeBlock
	// rpmSec is the time constant of the RPM smoothing, and controlSec
	// that of the throttle and load smoothing.
	rpmSec     = 0.012
	controlSec = 0.025
	// masterSec is the time constant of the fade when the engine starts
	// or stops, and silentLevel the gain, 120 dB down, at which a stopping
	// engine falls silent.
	masterSec   = 0.02
	silentLevel = 1e-6
	// handover is the time constant in seconds over which the pitch
	// returns from the limiter RPM to the smoothed RPM.
	handover = 0.017
	// engLevelBlocks is the number of control blocks, about 46 ms, over
	// which the engine level that the pops follow is the mean power.
	engLevelBlocks = 8
	// waverHz1 and waverHz2 are the rates of the two slow sines of the
	// final gear waver. Their ratio is not a simple fraction, so that the
	// waver does not repeat audibly.
	waverHz1 = 0.37
	waverHz2 = 1.13
	// overrunThrottle is the throttle below which an overrun counts for
	// the own pops.
	overrunThrottle = 0.05
)

// Control is the engine state for one Render call.
type Control struct {
	RPM, Throttle, Load float32
	Running, Overrun    bool
	// Final is true in the final gear. The limiter does not cut, and the
	// engine holds the limiter RPM.
	Final bool
	// LimiterEvents is the number of rev limiter events since the last
	// call.
	LimiterEvents int
	// Gear is the gear of the snapshot. A change ends a limiter bounce.
	Gear uint8
	// PopRequests is the number of pops that the game requests since the
	// last call, and PopStrength the strength of the latest, from 0 to 1.
	PopRequests int
	PopStrength float32
	// NoPops stops the own pops of a lift-off or a wind-down.
	NoPops bool
	// Character is the pop character: 0 for the profile character, and 1,
	// 2 and 3 for stock, sport and race.
	Character int
}

// Resources are the data of a profile that New does not parse: the cycle
// tables and the impulse response of each channel, 16-bit samples as
// engine-sim reads them.
type Resources struct {
	Bank             *cycles.Bank
	ImpulseResponses [][]int16
}

// Engine renders one engine. It allocates nothing after New.
type Engine struct {
	spec  *Spec
	live  *cycles.Live
	rt    *esynth.Runtime
	in    []float64
	side  []float64
	ctl   Control
	steps int // Simulation steps of the last control block.
	// nominal is the mean number of simulation steps of a control block,
	// and invNominal its reciprocal.
	nominal, invNominal float64

	// Smoothed controls. pitch is the RPM of the crank at the last step,
	// and pitchStep its change per step toward the pitch of the control
	// block.
	rpm, throttle, load float32
	pitch, pitchStep    float64
	pitchTo             float64
	kRPM, kCtl          float32
	master, kMaster     float32
	idle                idle

	// Rev limiter state, in seconds and RPM. cut is the time left of the
	// cut. cutting is true for a control block inside a cut. bounce is
	// true while the snapshot holds the engine at the limiter, and climb is
	// true while the internal RPM lrpm climbs back to the limiter RPM
	// after a cut of a bounce. dip is the pitch offset below the smoothed
	// RPM. bangAt is the time into a cut at which the unburnt fuel bangs,
	// or -1. gear is the last gear. The steps of the control block from
	// offFrom up to offTo have the ignition off.
	limRPM, limFall, limRise float32
	limCut, cut, bangAt      float64
	feedSec, stepRate        float64
	offFrom, offTo           int
	cutting, limEvent        bool
	bounce, climb            bool
	gear                     uint8
	lrpm, dip                float32
	kHand                    float32

	// Final gear waver. waver is the depth from the profile, and wavMix
	// fades it in and out at the limiter RPM. wav is the pitch offset of
	// the control block, from -waver to 0. wavPh1 and wavPh2 are the sine
	// phases in cycles, which advance by wavInc1 and wavInc2 per block.
	waver, wavMix, wav float32
	wavPh1, wavPh2     float32
	wavInc1, wavInc2   float32

	// Pops. level is the RMS level of the engine before the leveller over
	// the last engLevelBlocks control blocks, whose powers engRing holds
	// from engI on, engN of them filled. hold is the control blocks left in
	// which a pop that ended can still sound through the impulse response.
	pops       pops
	level      float64
	engRing    [engLevelBlocks]float32
	engI, engN int
	hold       int
	holdLen    int
	redline    float32
	noise      dsp.Noise
	channels   int
}

// New returns an engine for the spec and its resources at the output
// sample rate.
func New(s *Spec, res Resources, rate int) (*Engine, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if rate < 8000 || rate > 384000 {
		return nil, errors.New("sample rate is not from 8000 to 384000 Hz")
	}
	if res.Bank == nil {
		return nil, errors.New("no cycle tables")
	}
	t := res.Bank.Tables()
	if len(t.Channels) != len(s.Channels) || len(res.ImpulseResponses) != len(t.Channels) || len(t.Channels) > MaxChannels {
		return nil, fmt.Errorf("profile %s: %d channels, %d impulse responses and %d channels in the tables", s.Name, len(s.Channels), len(res.ImpulseResponses), len(t.Channels))
	}
	rt, err := newRuntime(t, res.ImpulseResponses, s.Volume, rate, false)
	if err != nil {
		return nil, err
	}
	feedHz := float64(synthRate) / block
	seed := uint32(s.Seed & math.MaxUint16)
	nominal := t.Rate / feedHz
	e := &Engine{
		spec: s,
		live: res.Bank.NewLive(cycles.LiveOptions{
			Lag:  cycles.ThrottleLag{Close: s.Throttle.CloseLagS, Open: s.Throttle.OpenLagS},
			Seed: uint64(seed) + 0x51ab,
		}),
		rt:         rt,
		in:         make([]float64, len(t.Channels)),
		side:       make([]float64, len(t.Channels)),
		channels:   len(t.Channels),
		nominal:    nominal,
		invNominal: 1 / nominal,
		kRPM:       dsp.TimeCoef(rpmSec, feedHz),
		kCtl:       dsp.TimeCoef(controlSec, feedHz),
		kMaster:    dsp.TimeCoef(masterSec, float64(rate)),
		kHand:      1 - dsp.TimeCoef(handover, feedHz),
		idle:       newIdle(s, seed, feedHz),
		limRPM:     float32(s.LimiterRPM),
		limFall:    float32(s.Limiter.FallRPMS * s.Limiter.LoadScale),
		limRise:    float32(s.Limiter.RiseRPMS * s.Limiter.LoadScale),
		limCut:     s.Limiter.CutMS * 1e-3,
		feedSec:    1 / feedHz,
		stepRate:   t.Rate,
		waver:      float32(s.Limiter.FinalGearWaver),
		wavInc1:    float32(waverHz1 / feedHz),
		wavInc2:    float32(waverHz2 / feedHz),
		pops:       newPops(s, t.Rate, 1/feedHz),
		redline:    float32(s.RedlineRPM),
		noise:      dsp.NewNoise(seed + 0x11a7),
		bangAt:     -1,
	}
	// A pop that ends can sound on through the longest impulse response
	// and the convolution block.
	taps := 0
	for _, ir := range res.ImpulseResponses {
		taps = max(taps, len(ir))
	}
	e.holdLen = int(math.Ceil(float64(taps+2*block)/block)) + 1
	gains, err := popGains(s, res, t)
	if err != nil {
		return nil, err
	}
	e.pops.kindGain = gains
	return e, nil
}

// newRuntime returns the synthesiser runtime of the tables at the output
// rate, with the audio parameters of the tables and the volume.
func newRuntime(t *cycles.Tables, irs [][]int16, volume float64, rate int, unlevelled bool) (*esynth.Runtime, error) {
	params := esynth.SimulatorParameters(len(t.Channels), t.Rate)
	vols := make([]float64, len(t.Channels))
	for i, c := range t.Channels {
		vols[i] = c.Volume
	}
	rt, err := esynth.NewRuntime(params, irs, vols, esynth.RuntimeOptions{OutputSampleRate: rate, Unlevelled: unlevelled, Side: !unlevelled}, esynth.NewRandom(1))
	if err != nil {
		return nil, err
	}
	audio := t.Audio
	audio.Volume = float64(float32(volume))
	rt.SetAudioParameters(audio)
	rt.SetInputSampleRate(t.Rate)
	return rt, nil
}

// PitchRPM returns the RPM that the crank follows at the last step. It
// differs from the smoothed input RPM in a limiter bounce, in the handover
// after it, in the final gear waver and in the idle wander.
func (e *Engine) PitchRPM() float32 { return float32(e.pitch) }

// Cutting reports whether the last control block was inside a limiter
// cut, with the ignition off.
func (e *Engine) Cutting() bool { return e.cutting }

// LevellerGain returns the gain of the engine-sim leveller after the last
// rendered sample.
func (e *Engine) LevellerGain() float32 { return e.rt.LevellerGain() }

// PopReport returns the number of pops started since New, and the kind,
// source and strength of the latest. The kinds count from 0 and the
// sources from 1, in the order of the snarl PopKind and PopSource values.
func (e *Engine) PopReport() (count uint32, kind, source int, strength float32) {
	p := &e.pops
	return p.total, p.lastKind, p.lastSrc, p.lastStrength
}

// Render fills dst at the output rate. It allocates nothing.
func (e *Engine) Render(dst []float32, c Control) {
	if c.LimiterEvents > 0 && !c.Final {
		e.limEvent = true
	}
	if c.PopRequests > 0 {
		e.pops.request(c.PopRequests, c.PopStrength)
	}
	e.ctl = c
	target := b2f(c.Running)
	k := 0
	for k < len(dst) {
		if !c.Running && e.master == 0 {
			// The fade has ended: the engine is silent and does not run.
			clear(dst[k:])
			return
		}
		n := e.rt.Render(dst[k:])
		for i := k; i < k+n; i++ {
			e.master += (target - e.master) * e.kMaster
			// The synthesiser clamps its output to the 16-bit range, and the
			// resampler can ring a little past it.
			dst[i] = min(1, max(-1, dst[i]*e.master))
		}
		if !c.Running && e.master < silentLevel {
			e.master = 0
		}
		k += n
		if k < len(dst) {
			e.feed()
		}
	}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func clampf(x, lo, hi float32) float32 {
	if x != x { // NaN.
		return lo
	}
	return min(hi, max(lo, x))
}

// feed runs one control block and writes its simulation steps, until the
// runtime holds the input of one convolution block.
func (e *Engine) feed() {
	e.control()
	steps := 0
	in := e.in
	for e.rt.InputSize() < block {
		e.pitch += e.pitchStep
		if (e.pitchStep > 0 && e.pitch > e.pitchTo) || (e.pitchStep < 0 && e.pitch < e.pitchTo) {
			e.pitch = e.pitchTo
		}
		e.live.Next(e.pitch, float64(e.throttle), steps < e.offFrom || steps >= e.offTo, in)
		e.pops.tick()
		// The pops are the side input of the synthesiser: they take every
		// stage of the engine input, and the leveller gain of the engine
		// alone, so that they do not duck the engine. The side input runs
		// on until the impulse response has rung out.
		if e.pops.active > 0 || e.hold > 0 {
			for c := range e.side {
				e.side[c] = 0
				if e.pops.active > 0 {
					e.side[c] = float64(e.pops.jump[c]) * e.level
				}
			}
			e.rt.WriteInputSide(in, e.side)
		} else {
			e.rt.WriteInput(in)
		}
		steps++
	}
	e.steps = steps
}

// control runs once per control block, before its simulation steps.
func (e *Engine) control() {
	c := &e.ctl
	rpm := clampf(c.RPM, 0, maxRPM)
	if c.Final && e.limRPM > 0 {
		// The final gear runs out of power at the limiter RPM.
		rpm = min(rpm, e.limRPM)
	}
	thr := clampf(c.Throttle, 0, 1)
	load := clampf(c.Load, 0, 1)
	if e.rpm == 0 && e.pitch == 0 {
		// The first block starts at the input RPM.
		e.rpm = rpm
	}
	e.rpm += (rpm - e.rpm) * e.kRPM
	e.throttle = thr
	e.load += (load - e.load) * e.kCtl
	limiting := e.limit(rpm, thr, c.Gear, c.Final)
	over := c.Overrun && thr < overrunThrottle && !limiting

	e.idle.control(rpm, e.load)
	pitch := float64(max(0, (e.rpm-e.dip)*(1+e.wav)*e.idle.speed))
	if e.pitch == 0 {
		e.pitch = pitch
	}
	e.pitchTo = pitch
	e.pitchStep = (pitch - e.pitch) * e.invNominal

	// The engine level for the pops: the mean power of the engine before
	// the leveller over the last engLevelBlocks blocks. The pops are the side
	// input, which the power leaves out.
	e.engRing[e.engI] = float32(e.rt.PreLevelPower())
	e.engI = (e.engI + 1) % engLevelBlocks
	e.engN = min(e.engN+1, engLevelBlocks)
	var sum float32
	for _, v := range e.engRing {
		sum += v
	}
	e.level = float64(math.Sqrt(float64(sum / float32(e.engN))))
	switch {
	case e.pops.active > 0 || e.pops.sounded:
		e.pops.sounded = false
		e.hold = e.holdLen
	case e.hold > 0:
		e.hold--
	}
	rr := min(e.rpm/e.redline, 1.2)
	heat := b2f(thr > 0.2) * e.load * min(rr, 1)
	e.pops.control(&popInput{
		rpm: e.rpm, throttle: thr, overrun: over && e.master > 0.5, cut: limiting, off: c.NoPops,
		heat: heat, character: c.Character, n: e.steps, paths: e.channels,
	})
}

// limit runs the rev limiter for a control block, from the input RPM,
// throttle and gear. It follows the engine-sim limiter: a cut turns the
// ignition off for the cut length while the internal RPM falls at the
// fall rate. After the cut, the engine fires and the internal RPM climbs
// at the rise rate to the limiter RPM, where the next cut starts. The
// bounce runs while the snapshot RPM is at the limiter with the throttle
// open. A limiter event starts one cut when no cut or climb runs. When
// the bounce or an event cut ends, the pitch returns to the smoothed RPM.
// It reports whether a cut or a climb runs, so that no overrun effect
// plays.
//
// In the final gear, no cut starts, and a cut or a climb in progress
// ends. At the limiter RPM with the throttle open, the pitch wavers slowly
// below the limiter RPM, so that the held tone does not sound fixed.
func (e *Engine) limit(rpm, thr float32, gear uint8, final bool) bool {
	e.waverStep(final && e.limRPM > 0 && rpm >= e.limRPM*0.998 && thr > 0.1)
	// The smoothed RPM must be near the limiter too, so that a bounce
	// starts from the limiter RPM.
	at := !final && e.limRPM > 0 && rpm >= e.limRPM*0.998 && e.rpm >= e.limRPM*0.99 && thr > 0.1 && gear == e.gear
	e.gear = gear
	if e.bounce && !at || final {
		e.bounce, e.climb, e.cut = false, false, 0
	}
	if !final && e.cut <= 0 && !e.climb && (at || e.limEvent) && e.limCut > 0 {
		e.lrpm = e.rpm - e.dip
		e.startCut(at)
	}
	e.limEvent = false
	e.offFrom, e.offTo = 0, 0
	if e.cut <= 0 && !e.climb {
		// Handover: the offset decays, so that the pitch returns to the
		// smoothed RPM with no step.
		e.dip *= e.kHand
		if e.dip < 1 && e.dip > -1 {
			e.dip = 0
		}
		e.cutting = false
		return false
	}
	// The cut and the climb run in time within the block, so that the
	// ignition turns off and on at the step where it would.
	t, from, to := 0.0, -1.0, -1.0
	for t < e.feedSec && (e.cut > 0 || e.climb) {
		if e.cut > 0 {
			part := min(e.cut, e.feedSec-t)
			if from < 0 {
				from = t
			}
			if e.bangAt >= 0 && e.limCut-e.cut+part >= e.bangAt {
				e.pops.bang(e.channels)
				e.bangAt = -1
			}
			e.cut -= part
			e.lrpm -= e.limFall * float32(part)
			t += part
			to = t
			e.climb = e.cut <= 0 && e.bounce
			continue
		}
		need := float64((e.limRPM - e.lrpm) / e.limRise)
		if need > e.feedSec-t {
			e.lrpm += e.limRise * float32(e.feedSec-t)
			break
		}
		e.lrpm, e.climb = e.limRPM, false
		t += need
		if !e.bounce {
			break
		}
		e.startCut(true)
	}
	if from >= 0 {
		e.offFrom = int(from*e.stepRate + 0.5)
		e.offTo = int(to*e.stepRate + 0.5)
	}
	e.cutting = to > from
	e.dip = e.rpm - e.lrpm
	return true
}

// startCut starts a cut of the limiter. bounce is true for a cut of a
// bounce at the limiter, and false for a cut of a limiter event.
func (e *Engine) startCut(bounce bool) {
	e.cut = e.limCut
	e.bounce = bounce
	e.bangAt = -1
	// A cut bangs with a chance of twice the pop density, at a random time
	// in the cut, when the profile has limiter bangs.
	if e.noise.Unipolar() < e.pops.bangChance {
		e.bangAt = (0.2 + 0.8*float64(e.noise.Unipolar())) * e.limCut
	}
}

// waverStep updates the final gear waver for one control block. top is
// true in the final gear at the limiter RPM with the throttle open. The
// waver fades in and out with the control time constant. It is the sum of
// two slow table sines, mapped from -waver to 0, so that the pitch never
// passes the limiter RPM.
func (e *Engine) waverStep(top bool) {
	if e.waver == 0 || !top && e.wavMix == 0 {
		return
	}
	e.wavMix += (b2f(top) - e.wavMix) * e.kCtl
	if !top && e.wavMix < 1e-4 {
		e.wavMix, e.wav = 0, 0
		return
	}
	e.wavPh1 += e.wavInc1
	if e.wavPh1 >= 1 {
		e.wavPh1--
	}
	e.wavPh2 += e.wavInc2
	if e.wavPh2 >= 1 {
		e.wavPh2--
	}
	w := 0.6*dsp.Sine(e.wavPh1) + 0.4*dsp.Sine(e.wavPh2)
	e.wav = e.waver * e.wavMix * (w - 1) * 0.5
}
