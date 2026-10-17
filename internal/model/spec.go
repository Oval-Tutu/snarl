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
	Name         string  `toml:"name"`
	Description  string  `toml:"description"`
	Seed         int     `toml:"seed"`
	IdleRPM      float64 `toml:"idle_rpm"`
	RedlineRPM   float64 `toml:"redline_rpm"`
	LimiterRPM   float64 `toml:"limiter_rpm"`
	LimiterKind  string  `toml:"limiter_kind"`
	LimiterMS    float64 `toml:"limiter_ms"`
	CamSwitchRPM float64 `toml:"cam_switch_rpm"`

	Pulse      PulseSpec      `toml:"pulse"`
	Exhaust    ExhaustSpec    `toml:"exhaust"`
	Noise      NoiseSpec      `toml:"noise"`
	Mechanical MechanicalSpec `toml:"mechanical"`
	Pops       PopSpec        `toml:"pops"`
	Overrun    OverrunSpec    `toml:"overrun"`
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
	// IdleFloor is the pulse gain at zero load, as a fraction of full load.
	IdleFloor float64 `toml:"idle_floor"`
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
	AM            float64 `toml:"am"`
	Air           float64 `toml:"air"`
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

// VoiceSpec is the output voicing.
type VoiceSpec struct {
	Gain float64 `toml:"gain"`
	// OutputDB is the fixed output gain in decibels. It calibrates the
	// tier 0 full-throttle sweep-wot scenario to -16 LUFS.
	OutputDB float64 `toml:"output_db"`
	// Tier1DB is the extra output gain of tier 1 in decibels. It
	// calibrates the tier 1 full-throttle sweep-wot scenario to -16 LUFS.
	Tier1DB float64 `toml:"tier1_db"`
}

// DefaultSpec returns the defaults that a profile file overrides.
func DefaultSpec() Spec {
	return Spec{
		Seed:        1,
		LimiterKind: "fuel",
		LimiterMS:   80,
		Pulse: PulseSpec{
			WidthDeg: 160, Attack: 25, Decay: 7, IdleFloor: 0.2,
			CycleVariation: 0.08, Jitter: 0.003,
			GainVariation: 0.06, TimingVariationDeg: 1.5,
		},
		Exhaust: ExhaustSpec{
			SpeedOfSound: 565, Crossover: "none", OutletDirect: 0.4,
			Steepening: 0.3, LowShelfHz: 250,
		},
		Noise: NoiseSpec{AM: 0.35, Air: 0.03, Jet: 0.02, JetHz: 1500},
		Mechanical: MechanicalSpec{
			TickHz: 3000,
		},
		Pops: PopSpec{
			MinRPM: 2500, Gain: 1, BodyHz: 200, BodyMS: 35, JumpMS: 3,
			Crack: 2.5, CrackMS: 16, IntervalM: 174,
		},
		Overrun: OverrunSpec{LevelDB: -1.5, IdleLevelDB: -4},
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
	for _, check := range []func() error{s.validateEngine, s.validateCylinders, s.validatePaths} {
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
	case s.LimiterKind != "fuel" && s.LimiterKind != "injector" && s.LimiterKind != "ignition":
		return fmt.Errorf("limiter_kind %q is not fuel, injector or ignition", s.LimiterKind)
	case s.Exhaust.Crossover != "none" && s.Exhaust.Crossover != "full" && s.Exhaust.Crossover != "partial":
		return fmt.Errorf("crossover %q is not none, full or partial", s.Exhaust.Crossover)
	case s.Pulse.WidthDeg <= 0 || s.Pulse.WidthDeg > 360 || s.Pulse.Attack <= 0 || s.Pulse.Decay <= 0:
		return errors.New("pulse width_deg must be up to 360, and width_deg, attack and decay positive")
	case s.Exhaust.SpeedOfSound <= 0 || s.Exhaust.Coupling < 0 || s.Exhaust.Coupling > 0.5:
		return errors.New("speed_of_sound must be positive and coupling from 0 to 0.5")
	case s.Pops.BodyHz <= 0 || s.Pops.BodyMS <= 0 || s.Pops.CrackMS <= 0 || s.Pops.IntervalM <= 0 || s.Pops.JumpMS <= 0:
		return errors.New("pops body_hz, body_ms, jump_ms, crack_ms and interval_ms must be positive")
	case s.Voice.Gain <= 0 || math.Abs(s.Voice.OutputDB) > 40 || math.Abs(s.Voice.Tier1DB) > 20:
		return errors.New("voice gain must be positive, output_db from -40 to 40 and tier1_db from -20 to 20")
	case math.Abs(s.Exhaust.LowShelfDB) > 20 || s.Exhaust.LowShelfHz <= 0:
		return errors.New("exhaust low_shelf_db must be from -20 to 20 and low_shelf_hz positive")
	case s.Overrun.LevelDB > 6 || s.Overrun.IdleLevelDB > 6 || s.Overrun.LevelDB < -40 || s.Overrun.IdleLevelDB < -40:
		return errors.New("overrun level_db and idle_level_db must be from -40 to 6")
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
