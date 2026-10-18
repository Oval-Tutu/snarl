// Package model is the engine synthesis model: crank, firing schedule,
// pulses, exhaust paths, noise layers, pops and the output stage.
package model

import (
	"errors"
	"fmt"
	"math"
)

// MaxCylinders is the largest cylinder count that a profile can have.
const MaxCylinders = 16

// MaxBanks is the largest bank count that a profile can have.
const MaxBanks = 4

// Spec is the parsed content of an engine profile. The toml tags name the
// keys in the profile files.
type Spec struct {
	Name        string  `toml:"name"`
	Description string  `toml:"description"`
	Seed        int     `toml:"seed"`
	IdleRPM     float64 `toml:"idle_rpm"`
	RedlineRPM  float64 `toml:"redline_rpm"`
	// LimiterRPM is the snapshot RPM at which the rev limiter cuts with
	// the throttle open. Zero turns the limiter off.
	LimiterRPM float64 `toml:"limiter_rpm"`
	// LimiterKind is the cut: "fuel" stops the firing of every cylinder,
	// and "ignition" stops it with bangs from the unburnt fuel.
	LimiterKind string `toml:"limiter_kind"`
	// LimiterMS is the length of one cut in milliseconds.
	LimiterMS float64 `toml:"limiter_ms"`
	// LimiterJitter is the random spread of the cut length. Each cut is
	// LimiterMS times a random factor from 1-LimiterJitter to
	// 1+LimiterJitter, drawn once per cut. Zero gives a regular bounce.
	LimiterJitter float64 `toml:"limiter_jitter"`
	// LimiterFallRPMS is the rate in RPM per second at which the engine
	// slows down during a cut, from friction and pumping. The dip of one
	// bounce is this rate times the cut length.
	LimiterFallRPMS float64 `toml:"limiter_fall_rpm_s"`
	// LimiterRiseRPMS is the rate in RPM per second at which the engine
	// climbs back to the limiter RPM after a cut. The next cut starts when
	// it gets there, so one bounce takes LimiterMS plus the dip divided by
	// this rate.
	LimiterRiseRPMS float64 `toml:"limiter_rise_rpm_s"`
	// LimiterLoadScale multiplies both rates, for an engine in gear. The
	// dip grows with it, and the bounce rate stays the same.
	LimiterLoadScale float64 `toml:"limiter_load_scale"`
	// LimiterPumpDB is the level in decibels of the pulse of each cylinder
	// in a cut, relative to a firing. A cylinder that does not fire still
	// pumps its charge into the exhaust at its firing time.
	LimiterPumpDB float64 `toml:"limiter_pump_db"`
	// LimiterNotchDB is the lowest level in decibels of the pumping pulses,
	// relative to a firing. The level falls to it along a raised cosine
	// over 35 ms after the start of a cut and returns to LimiterPumpDB
	// over the next 35 ms.
	LimiterNotchDB float64 `toml:"limiter_notch_db"`
	// LimiterBarkDB is the extra gain in decibels of the first firings
	// after a cut. It decays with a time constant of 10 ms.
	LimiterBarkDB float64 `toml:"limiter_bark_db"`
	CamSwitchRPM  float64 `toml:"cam_switch_rpm"`

	Pulse      PulseSpec      `toml:"pulse"`
	Exhaust    ExhaustSpec    `toml:"exhaust"`
	Noise      NoiseSpec      `toml:"noise"`
	Mechanical MechanicalSpec `toml:"mechanical"`
	Pops       PopSpec        `toml:"pops"`
	Overrun    OverrunSpec    `toml:"overrun"`
	Idle       IdleSpec       `toml:"idle"`
	Voice      VoiceSpec      `toml:"voice"`
	Cylinders  []CylinderSpec `toml:"cylinder"`
	Paths      []PathSpec     `toml:"path"`
}

// PulseSpec is the shape of one exhaust pulse.
type PulseSpec struct {
	// WidthDeg is the pulse length in crank degrees.
	WidthDeg float64 `toml:"width_deg"`
	// Attack and Decay are the rates a and b of the envelope
	// (1-exp(-a x))exp(-b x), where x runs from 0 to 1 over the width.
	Attack float64 `toml:"attack"`
	Decay  float64 `toml:"decay"`
	// CamAttack replaces Attack above the cam switch RPM.
	CamAttack float64 `toml:"cam_attack"`
	// CamGain is the extra pulse gain above the cam switch RPM.
	CamGain float64 `toml:"cam_gain"`
	// IdleFloor is the reference pulse gain for the overrun air noise and
	// pops, as a fraction of full load. On overrun, both rise by the
	// overrun level over this gain.
	IdleFloor float64 `toml:"idle_floor"`
	// PartLoadDB is the pulse level at load 0.5 in decibels, relative to
	// full load at the same RPM. The pulse level rises smoothly with load
	// from the overrun level at load 0 to full load at load 1.
	PartLoadDB float64 `toml:"part_load_db"`
	// CycleVariation is the random per-firing gain spread.
	CycleVariation float64 `toml:"cycle_variation"`
	// Jitter is the slow random RPM and gain wander, as a fraction.
	Jitter float64 `toml:"jitter"`
	// GainVariation is the fixed per-cylinder gain spread.
	GainVariation float64 `toml:"gain_variation"`
	// TimingVariationDeg is the fixed per-cylinder timing spread in
	// crank degrees.
	TimingVariationDeg float64 `toml:"timing_variation_deg"`
}

// ExhaustSpec holds the settings that all exhaust paths share.
type ExhaustSpec struct {
	// SpeedOfSound is the hot gas speed of sound in metres per second.
	SpeedOfSound float64 `toml:"speed_of_sound"`
	// Crossover is "none", "full" or "partial".
	Crossover string `toml:"crossover"`
	// Coupling is the share of the other bank in each path for a partial
	// crossover, from 0 to 0.5.
	Coupling float64 `toml:"coupling"`
	// OutletDirect is the gain of the low-frequency pressure that passes
	// the outlet next to the radiation filter.
	OutletDirect float64 `toml:"outlet_direct"`
	// Steepening is the load-dependent wave steepening (tier 1).
	Steepening float64 `toml:"steepening"`
	// Overlap is the gain of the quadratic pressure term in each bank
	// collector, before the crossover. Overlapping pulses of one bank add
	// a pressure peak, which keeps the per-bank firing pattern audible.
	Overlap float64 `toml:"overlap"`
	// LowShelfDB is the low-frequency gain of the exhaust body in
	// decibels, below LowShelfHz. A boost gives the sound
	// weight.
	LowShelfDB float64 `toml:"low_shelf_db"`
	LowShelfHz float64 `toml:"low_shelf_hz"`
}

// CylinderSpec is one cylinder.
type CylinderSpec struct {
	// CrankDeg is the firing angle in the 720-degree cycle.
	CrankDeg float64 `toml:"crank_deg"`
	// Bank is the exhaust bank, from 0.
	Bank int `toml:"bank"`
	// PrimaryM is the primary pipe length in metres.
	PrimaryM float64 `toml:"primary_m"`
}

// PathSpec is one exhaust path from a bank collector to an outlet.
type PathSpec struct {
	CollectorM  float64 `toml:"collector_m"`
	Reflection  float64 `toml:"reflection"`
	DampingHz   float64 `toml:"damping_hz"`
	PipeRadiusM float64 `toml:"pipe_radius_m"`
	OutletM     float64 `toml:"outlet_radius_m"`
	WallLossDB  float64 `toml:"wall_loss_db"`
	WallLossHz  float64 `toml:"wall_loss_hz"`
	ChamberM    float64 `toml:"chamber_m"`
	ChamberArea float64 `toml:"chamber_area_ratio"`
	Gain        float64 `toml:"gain"`
	// ListenerM is the extra distance in metres from this outlet to the
	// listener, through air at 343 metres per second.
	ListenerM float64 `toml:"listener_m"`
}

// NoiseSpec holds the noise layer gains.
type NoiseSpec struct {
	AM  float64 `toml:"am"`
	Air float64 `toml:"air"`
	// AirCutoffHz is the cutoff of each of the two one-pole low passes on
	// the overrun air noise.
	AirCutoffHz   float64 `toml:"air_cutoff_hz"`
	Jet           float64 `toml:"jet"`
	JetHz         float64 `toml:"jet_hz"`
	Intake        float64 `toml:"intake"`
	IntakeHz      float64 `toml:"intake_hz"`
	IntakeCamGain float64 `toml:"intake_cam_gain"`
	IntakeCamHz   float64 `toml:"intake_cam_hz"`
}

// MechanicalSpec is the mechanical layer (tier 1).
type MechanicalSpec struct {
	FanRatio  float64 `toml:"fan_ratio"`
	FanBlades float64 `toml:"fan_blades"`
	FanGain   float64 `toml:"fan_gain"`
	TickGain  float64 `toml:"tick_gain"`
	TickHz    float64 `toml:"tick_hz"`
}

// PopSpec is the overrun pop model.
type PopSpec struct {
	Density   float64 `toml:"density"`
	MinRPM    float64 `toml:"min_rpm"`
	Gain      float64 `toml:"gain"`
	BodyHz    float64 `toml:"body_hz"`
	BodyMS    float64 `toml:"body_ms"`
	JumpMS    float64 `toml:"jump_ms"`
	Crack     float64 `toml:"crack"`
	CrackMS   float64 `toml:"crack_ms"`
	IntervalM float64 `toml:"interval_ms"`
	// The weights of the pop kinds. Each pop picks a kind at random in
	// proportion to its weight.
	WeightPop     float64 `toml:"weight_pop"`
	WeightBoom    float64 `toml:"weight_boom"`
	WeightCrackle float64 `toml:"weight_crackle"`
	WeightBurble  float64 `toml:"weight_burble"`
}

// OverrunSpec is the output level on closed throttle. The engine model
// sets the pulse amplitude on overrun to this level.
type OverrunSpec struct {
	// LevelDB is the output level on closed throttle at the redline, in
	// decibels relative to full throttle at the same RPM.
	LevelDB float64 `toml:"level_db"`
	// IdleLevelDB is the output level on closed throttle at idle, in
	// decibels relative to full throttle at the same RPM. The level
	// changes linearly with RPM from idle to the redline.
	IdleLevelDB float64 `toml:"idle_level_db"`
}

// IdleSpec is the low-RPM character: combustion variation, cylinder
// imbalance, a slow idle hunt and a slow rocking. Each effect has its full
// depth at idle and falls along a smoothstep over RPM to zero at FadeRPM.
// The variation, hunt and rocking also fall linearly with load to zero at
// full load. Above FadeRPM, the output does not change with these
// settings.
type IdleSpec struct {
	// FadeRPM is the RPM at which the effects reach zero.
	FadeRPM float64 `toml:"fade_rpm"`
	// Variation is the extra random per-firing gain spread.
	Variation float64 `toml:"variation"`
	// TimingDeg is the extra random per-firing delay spread in crank
	// degrees.
	TimingDeg float64 `toml:"timing_deg"`
	// Imbalance is the extra fixed per-cylinder gain spread. It scales the
	// gain_variation pattern and adds half-order content.
	Imbalance float64 `toml:"imbalance"`
	// Hunt is the depth of the slow RPM modulation, as a fraction of RPM.
	Hunt float64 `toml:"hunt"`
	// Rock is the depth of the slow pulse amplitude modulation.
	Rock float64 `toml:"rock"`
	// HuntHz is the rate of the hunt and the rocking in hertz. A slow
	// random wander makes the modulation irregular.
	HuntHz float64 `toml:"hunt_hz"`
}

// VoiceSpec is the output voicing.
type VoiceSpec struct {
	Gain float64 `toml:"gain"`
	// OutputDB is the fixed output gain in decibels. It calibrates the
	// tier 0 full-throttle sweep-wot scenario to -16 LUFS.
	OutputDB float64 `toml:"output_db"`
	// Tier1DB is the extra output gain of tier 1 in decibels. It
	// calibrates the tier 1 full-throttle sweep-wot scenario to -16 LUFS.
	Tier1DB float64 `toml:"tier1_db"`
	// IdleGainDB is the extra output gain in decibels at and below
	// IdleGainRPM, for all loads. Above IdleGainRPM, it falls along a
	// smoothstep to 0 dB at the redline. A positive value makes the level
	// fall less as the RPM falls. IdleGainRPM 0 selects the idle RPM.
	IdleGainDB  float64 `toml:"idle_gain_db"`
	IdleGainRPM float64 `toml:"idle_gain_rpm"`
	// Tier1IdleGainDB is added to IdleGainDB in tier 1, because the two
	// tiers change differently in level over RPM.
	Tier1IdleGainDB float64 `toml:"tier1_idle_gain_db"`
}

// DefaultSpec returns the defaults that a profile file overrides.
func DefaultSpec() Spec {
	return Spec{
		Seed:             1,
		LimiterKind:      "fuel",
		LimiterMS:        50,
		LimiterFallRPMS:  6000,
		LimiterRiseRPMS:  10000,
		LimiterLoadScale: 1,
		LimiterPumpDB:    -1,
		LimiterNotchDB:   -3,
		LimiterBarkDB:    1,
		Pulse: PulseSpec{
			WidthDeg: 160, Attack: 25, Decay: 7, IdleFloor: 0.2, PartLoadDB: -0.6,
			CycleVariation: 0.08, Jitter: 0.003,
			GainVariation: 0.06, TimingVariationDeg: 1.5,
		},
		Exhaust: ExhaustSpec{
			SpeedOfSound: 565, Crossover: "none", OutletDirect: 0.4,
			Steepening: 0.3, LowShelfHz: 250,
		},
		Noise: NoiseSpec{AM: 0.35, Air: 0.03, AirCutoffHz: 900, Jet: 0.02, JetHz: 1500},
		Mechanical: MechanicalSpec{
			TickHz: 3000,
		},
		Pops: PopSpec{
			MinRPM: 2500, Gain: 1, BodyHz: 200, BodyMS: 35, JumpMS: 3,
			Crack: 2.5, CrackMS: 16, IntervalM: 174,
			// The kind mix of the muffler crackle pack.
			WeightPop: 0.4, WeightBoom: 0.2, WeightCrackle: 0.3, WeightBurble: 0.1,
		},
		Overrun: OverrunSpec{LevelDB: -1.2, IdleLevelDB: -1.2},
		Idle:    IdleSpec{FadeRPM: 2200, HuntHz: 1.2},
		Voice:   VoiceSpec{Gain: 1},
	}
}

// Banks returns the bank count.
func (s *Spec) Banks() int {
	n := 0
	for _, c := range s.Cylinders {
		n = max(n, c.Bank+1)
	}
	return n
}

// Validate reports the first problem in the spec.
func (s *Spec) Validate() error {
	if s.Name == "" {
		return errors.New("profile has no name")
	}
	for _, check := range []func() error{s.validateEngine, s.validateLimiter, s.validateIdle, s.validateCylinders, s.validatePaths} {
		if err := check(); err != nil {
			return fmt.Errorf("profile %s: %w", s.Name, err)
		}
	}
	return nil
}

func (s *Spec) validateEngine() error {
	switch {
	case s.Seed < 0 || s.Seed > math.MaxUint16:
		return errors.New("seed must be from 0 to 65535")
	case s.IdleRPM <= 0 || s.RedlineRPM <= s.IdleRPM || s.LimiterMS < 0:
		return errors.New("idle_rpm, redline_rpm and limiter_ms must be positive, and redline above idle")
	case s.Exhaust.Crossover != "none" && s.Exhaust.Crossover != "full" && s.Exhaust.Crossover != "partial":
		return fmt.Errorf("crossover %q is not none, full or partial", s.Exhaust.Crossover)
	case s.Pulse.WidthDeg <= 0 || s.Pulse.WidthDeg > 360 || s.Pulse.Attack <= 0 || s.Pulse.Decay <= 0:
		return errors.New("pulse width_deg must be up to 360, and width_deg, attack and decay positive")
	case s.Exhaust.SpeedOfSound <= 0 || s.Exhaust.Coupling < 0 || s.Exhaust.Coupling > 0.5:
		return errors.New("speed_of_sound must be positive and coupling from 0 to 0.5")
	case s.Pops.BodyHz <= 0 || s.Pops.BodyMS <= 0 || s.Pops.CrackMS <= 0 || s.Pops.IntervalM <= 0 || s.Pops.JumpMS <= 0:
		return errors.New("pops body_hz, body_ms, jump_ms, crack_ms and interval_ms must be positive")
	case min(s.Pops.WeightPop, s.Pops.WeightBoom, s.Pops.WeightCrackle, s.Pops.WeightBurble) < 0 ||
		s.Pops.WeightPop+s.Pops.WeightBoom+s.Pops.WeightCrackle+s.Pops.WeightBurble <= 0:
		return errors.New("pops weights must not be negative, and at least one must be positive")
	case s.Voice.Gain <= 0 || math.Abs(s.Voice.OutputDB) > 40 || math.Abs(s.Voice.Tier1DB) > 20:
		return errors.New("voice gain must be positive, output_db from -40 to 40 and tier1_db from -20 to 20")
	case math.Abs(s.Voice.IdleGainDB) > 20 || math.Abs(s.Voice.IdleGainDB+s.Voice.Tier1IdleGainDB) > 20 ||
		s.Voice.IdleGainRPM < 0 || s.Voice.IdleGainRPM >= s.RedlineRPM:
		return errors.New("voice idle_gain_db must be from -20 to 20 in both tiers, and idle_gain_rpm from 0 to below redline_rpm")
	case math.Abs(s.Exhaust.LowShelfDB) > 20 || s.Exhaust.LowShelfHz <= 0:
		return errors.New("exhaust low_shelf_db must be from -20 to 20 and low_shelf_hz positive")
	case s.Overrun.LevelDB > 6 || s.Overrun.IdleLevelDB > 6 || s.Overrun.LevelDB < -40 || s.Overrun.IdleLevelDB < -40:
		return errors.New("overrun level_db and idle_level_db must be from -40 to 6")
	case s.Pulse.PartLoadDB >= 0 || s.Pulse.PartLoadDB < -20:
		return errors.New("pulse part_load_db must be from -20 to below 0")
	case s.Noise.AirCutoffHz < 20 || s.Noise.AirCutoffHz > 20000:
		return errors.New("noise air_cutoff_hz must be from 20 to 20000")
	}
	return nil
}

func (s *Spec) validateLimiter() error {
	switch {
	case s.LimiterKind != "fuel" && s.LimiterKind != "ignition":
		return fmt.Errorf("limiter_kind %q is not fuel or ignition", s.LimiterKind)
	case s.LimiterPumpDB > 0 || s.LimiterPumpDB < -60 || s.LimiterNotchDB > s.LimiterPumpDB || s.LimiterNotchDB < -60 ||
		s.LimiterBarkDB < 0 || s.LimiterBarkDB > 12:
		return errors.New("limiter_pump_db must be from -60 to 0, limiter_notch_db from -60 to limiter_pump_db and limiter_bark_db from 0 to 12")
	case s.LimiterRPM < 0 || s.LimiterMS > 500 || s.LimiterFallRPMS < 0 || s.LimiterFallRPMS > 100000 ||
		s.LimiterRiseRPMS <= 0 || s.LimiterRiseRPMS > 100000 || s.LimiterLoadScale < 0.1 || s.LimiterLoadScale > 10 ||
		s.LimiterJitter < 0 || s.LimiterJitter > 0.5:
		return errors.New("limiter_rpm must not be negative, limiter_ms from 0 to 500, limiter_fall_rpm_s from 0 to 100000, limiter_rise_rpm_s from above 0 to 100000, limiter_load_scale from 0.1 to 10 and limiter_jitter from 0 to 0.5")
	case s.LimiterMS*(1+s.LimiterJitter)*1e-3*s.LimiterFallRPMS*s.LimiterLoadScale >= s.LimiterRPM && s.LimiterRPM > 0:
		return errors.New("the longest limiter dip, limiter_ms times 1+limiter_jitter, limiter_fall_rpm_s and limiter_load_scale, must stay below limiter_rpm")
	}
	return nil
}

func (s *Spec) validateIdle() error {
	d := s.Idle
	switch {
	case d.FadeRPM <= s.IdleRPM || d.HuntHz <= 0 || d.HuntHz > 10:
		return errors.New("idle fade_rpm must be above idle_rpm and hunt_hz from above 0 to 10")
	case d.Variation < 0 || d.Variation > 1 || d.Imbalance < 0 || d.Imbalance > 1 ||
		d.Hunt < 0 || d.Hunt > 0.2 || d.Rock < 0 || d.Rock > 1 || d.TimingDeg < 0 || d.TimingDeg > 90:
		return errors.New("idle variation, imbalance and rock must be from 0 to 1, hunt from 0 to 0.2 and timing_deg from 0 to 90")
	}
	return nil
}

func (s *Spec) validateCylinders() error {
	n := len(s.Cylinders)
	if n == 0 || n > MaxCylinders {
		return fmt.Errorf("%d cylinders, want 1 to %d", n, MaxCylinders)
	}
	var used [MaxBanks]bool
	for i, c := range s.Cylinders {
		if c.CrankDeg < 0 || c.CrankDeg >= 720 || c.Bank < 0 || c.Bank >= MaxBanks || c.PrimaryM < 0 {
			return fmt.Errorf("cylinder %d has a bad crank_deg, bank or primary_m", i+1)
		}
		used[c.Bank] = true
		for j, d := range s.Cylinders[:i] {
			if g := math.Abs(c.CrankDeg - d.CrankDeg); g < 5 || g > 715 {
				return fmt.Errorf("cylinders %d and %d fire less than 5 degrees apart", j+1, i+1)
			}
		}
	}
	for b := range s.Banks() {
		if !used[b] {
			return fmt.Errorf("bank %d has no cylinder", b)
		}
	}
	return nil
}

func (s *Spec) validatePaths() error {
	if len(s.Paths) != s.Banks() {
		return fmt.Errorf("%d paths for %d banks", len(s.Paths), s.Banks())
	}
	for i, p := range s.Paths {
		if p.CollectorM <= 0 || p.OutletM <= 0 || p.PipeRadiusM <= 0 ||
			p.Reflection <= -1 || p.Reflection >= 1 || p.Gain <= 0 || p.ListenerM < 0 || p.ListenerM > 20 {
			return fmt.Errorf("path %d has a bad collector, radius, reflection, gain or listener distance", i+1)
		}
	}
	return nil
}
