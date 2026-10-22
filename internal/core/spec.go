// Package core is the snarl engine: engine-sim cycle tables played at the
// crank angle of the snapshot RPM into the real-time engine-sim
// synthesiser, with the intake lag, the limiter cut, the idle wander and
// the snarl game features (docs/scoping.md section 2).
package core

import (
	"errors"
	"fmt"
	"math"
)

// MaxChannels is the largest exhaust channel count of the tables.
const MaxChannels = 4

// Spec is the parsed content of an engine profile. The toml tags name the
// keys in the profile files.
type Spec struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	// Seed seeds the random sources of the engine.
	Seed int `toml:"seed"`
	// Cylinders is the cylinder count of the upstream engine.
	Cylinders  int     `toml:"cylinders"`
	IdleRPM    float64 `toml:"idle_rpm"`
	RedlineRPM float64 `toml:"redline_rpm"`
	// LimiterRPM is the snapshot RPM at which the rev limiter cuts with the
	// throttle open. Zero turns the limiter off.
	LimiterRPM float64 `toml:"limiter_rpm"`
	// Tables names the cycle tables, profiles/cycles/<tables>.bin.
	Tables string `toml:"tables"`
	// Volume is the synthesiser volume after the leveller. At the
	// engine-sim leveller target, 0.25 puts the engine peaks near -9 dBFS,
	// which leaves the pops room above the engine.
	Volume float64 `toml:"volume"`

	Throttle ThrottleSpec  `toml:"throttle"`
	Limiter  LimiterSpec   `toml:"limiter"`
	Idle     IdleSpec      `toml:"idle"`
	Pops     PopSpec       `toml:"pops"`
	Channels []ChannelSpec `toml:"channel"`
}

// ThrottleSpec is the intake lag: after a throttle change, the throttle
// rows of the tables move to their new weights with a first-order lag.
type ThrottleSpec struct {
	// CloseLagS and OpenLagS are the time constants in seconds of a
	// closing and an opening throttle.
	CloseLagS float64 `toml:"close_lag_s"`
	OpenLagS  float64 `toml:"open_lag_s"`
}

// LimiterSpec is the rev limiter: an ignition cut, as in engine-sim. A cut
// plays the ignition-off tables.
type LimiterSpec struct {
	// CutMS is the length of one cut in milliseconds.
	CutMS float64 `toml:"cut_ms"`
	// FallRPMS is the rate in RPM per second at which the engine slows down
	// during a cut. The dip of one bounce is this rate times the cut.
	FallRPMS float64 `toml:"fall_rpm_s"`
	// RiseRPMS is the rate in RPM per second at which the engine climbs back
	// to the limiter RPM after a cut. The next cut starts when it gets
	// there.
	RiseRPMS float64 `toml:"rise_rpm_s"`
	// LoadScale multiplies both rates, for an engine in gear.
	LoadScale float64 `toml:"load_scale"`
	// FinalGearWaver is the depth of the slow pitch waver in the final gear
	// at the limiter RPM, as a share of the RPM. Zero gives a fixed pitch.
	FinalGearWaver float64 `toml:"final_gear_waver"`
}

// IdleSpec is the idle wander: a slow RPM modulation at idle. It has its
// full depth at idle and falls along a smoothstep over RPM to zero at
// FadeRPM. It also falls linearly with load to zero at half load.
type IdleSpec struct {
	// FadeRPM is the RPM at which the wander reaches zero.
	FadeRPM float64 `toml:"fade_rpm"`
	// Hunt is the depth of the modulation, as a fraction of RPM.
	Hunt float64 `toml:"hunt"`
	// HuntHz is the rate of the modulation in hertz. A slow random wander
	// makes it irregular.
	HuntHz float64 `toml:"hunt_hz"`
}

// PopSpec is the pop model. A pop is a voice of the crackle model
// (internal/crackle), which joins the synthesiser output after the
// leveller.
type PopSpec struct {
	// Density scales the chances of the own pops of a lift-off and a
	// wind-down: 5 times the density multiplies each chance. Zero stops
	// the own pops and the limiter bangs. Requested pops do not depend on
	// it.
	Density float64 `toml:"density"`
	// Character is "stock", "sport" or "race". It scales the own pops:
	// their chances by 0.5, 1 or 1.8 and their gain by -6, 0 or +3 dB. The
	// requested pops change by -2, 0 or +1 dB.
	Character string `toml:"character"`
	// Gain multiplies every pop. At 1, each kind has its level over the
	// engine level.
	Gain float64 `toml:"gain"`
	// BodyMS sets the slot length of a pop and a limiter bang. The crackle
	// model sets the sound of the pops.
	BodyMS float64 `toml:"body_ms"`
	// LimiterBangs makes a limiter cut bang with a chance of twice the
	// density.
	LimiterBangs bool `toml:"limiter_bangs"`
	// The weights of the pop kinds. Each pop picks a kind at random in
	// proportion to its weight.
	WeightPop     float64 `toml:"weight_pop"`
	WeightBoom    float64 `toml:"weight_boom"`
	WeightCrackle float64 `toml:"weight_crackle"`
	WeightBurble  float64 `toml:"weight_burble"`
}

// ChannelSpec is one exhaust channel of the tables.
type ChannelSpec struct {
	// ImpulseResponse names the impulse response file of the channel in
	// profiles/ir.
	ImpulseResponse string `toml:"impulse_response"`
}

// DefaultSpec returns the defaults that a profile file overrides.
func DefaultSpec() Spec {
	return Spec{
		Seed:    1,
		Volume:  0.25,
		Limiter: LimiterSpec{CutMS: 50, FallRPMS: 6000, RiseRPMS: 10000, LoadScale: 1, FinalGearWaver: 0.003},
		Idle:    IdleSpec{FadeRPM: 2200, HuntHz: 1.2},
		Pops: PopSpec{
			Character: "sport", Gain: 1, BodyMS: 35,
			// The kind mix of the muffler crackle pack.
			WeightPop: 0.4, WeightBoom: 0.2, WeightCrackle: 0.3, WeightBurble: 0.1,
		},
	}
}

// Validate reports the first problem in the spec.
func (s *Spec) Validate() error {
	if s.Name == "" {
		return errors.New("profile has no name")
	}
	for _, check := range []func() error{s.validateEngine, s.validateLimiter, s.validateIdle, s.validatePops} {
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
	case s.Cylinders < 1 || s.Cylinders > 32:
		return errors.New("cylinders must be from 1 to 32")
	case s.IdleRPM <= 0 || s.RedlineRPM <= s.IdleRPM:
		return errors.New("idle_rpm and redline_rpm must be positive, and redline above idle")
	case s.Tables == "":
		return errors.New("profile names no cycle tables")
	case len(s.Channels) < 1 || len(s.Channels) > MaxChannels:
		return fmt.Errorf("%d channels, want 1 to %d", len(s.Channels), MaxChannels)
	case !(s.Volume > 0) || s.Volume > 1:
		return errors.New("volume must be from above 0 to 1")
	case s.Throttle.CloseLagS < 0 || s.Throttle.CloseLagS > 2 || s.Throttle.OpenLagS < 0 || s.Throttle.OpenLagS > 2:
		return errors.New("throttle close_lag_s and open_lag_s must be from 0 to 2")
	}
	for i, c := range s.Channels {
		if c.ImpulseResponse == "" {
			return fmt.Errorf("channel %d names no impulse response", i+1)
		}
	}
	return nil
}

func (s *Spec) validateLimiter() error {
	l := s.Limiter
	switch {
	case s.LimiterRPM < 0 || l.CutMS < 0 || l.CutMS > 500 || l.FallRPMS < 0 || l.FallRPMS > 100000 ||
		l.RiseRPMS <= 0 || l.RiseRPMS > 100000 || l.LoadScale < 0.1 || l.LoadScale > 10:
		return errors.New("limiter_rpm must not be negative, limiter cut_ms from 0 to 500, fall_rpm_s from 0 to 100000, rise_rpm_s from above 0 to 100000 and load_scale from 0.1 to 10")
	case l.CutMS*1e-3*l.FallRPMS*l.LoadScale >= s.LimiterRPM && s.LimiterRPM > 0:
		return errors.New("the limiter dip, cut_ms times fall_rpm_s and load_scale, must stay below limiter_rpm")
	case l.FinalGearWaver < 0 || l.FinalGearWaver > 0.05:
		return errors.New("limiter final_gear_waver must be from 0 to 0.05")
	}
	return nil
}

func (s *Spec) validateIdle() error {
	d := s.Idle
	switch {
	case d.FadeRPM <= s.IdleRPM || d.HuntHz <= 0 || d.HuntHz > 10:
		return errors.New("idle fade_rpm must be above idle_rpm and hunt_hz from above 0 to 10")
	case d.Hunt < 0 || d.Hunt > 0.2:
		return errors.New("idle hunt must be from 0 to 0.2")
	}
	return nil
}

func (s *Spec) validatePops() error {
	p := s.Pops
	switch {
	case p.BodyMS <= 0 || p.BodyMS > 500:
		return errors.New("pops body_ms must be from above 0 to 500")
	case p.Density < 0 || p.Gain < 0 || p.Gain > 10:
		return errors.New("pops density must not be negative and gain must be from 0 to 10")
	case !validCharacter(p.Character):
		return fmt.Errorf("pops character %q is not stock, sport or race", p.Character)
	case min(p.WeightPop, p.WeightBoom, p.WeightCrackle, p.WeightBurble) < 0 ||
		p.WeightPop+p.WeightBoom+p.WeightCrackle+p.WeightBurble <= 0:
		return errors.New("pops weights must not be negative, and at least one must be positive")
	}
	return nil
}

func validCharacter(name string) bool {
	_, ok := characterIndex(name)
	return ok
}
