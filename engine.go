package snarl

import (
	"embed"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync/atomic"

	"github.com/oval-tutu/snarl/internal/model"
)

//go:embed profiles/*.toml
var profileFiles embed.FS

// Profile is a parsed engine profile.
type Profile struct {
	// Name is the profile identifier, for example "flat12".
	Name string
	// Description names the engine type.
	Description string
	// IdleRPM, RedlineRPM and LimiterRPM are the engine speeds of the
	// profile, in revolutions per minute.
	IdleRPM, RedlineRPM, LimiterRPM float32

	spec *model.Spec
}

// ErrUnknownProfile is returned when no embedded profile has the name.
var ErrUnknownProfile = errors.New("unknown engine profile")

// Profiles returns the names of the embedded engine profiles.
func Profiles() []string {
	return []string{"flat12", "inline4", "flat6", "v8"}
}

// LoadProfile returns the embedded engine profile with the given name.
func LoadProfile(name string) (Profile, error) {
	known := false
	for _, n := range Profiles() {
		known = known || n == name
	}
	if !known {
		return Profile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	data, err := profileFiles.ReadFile("profiles/" + name + ".toml")
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	return parseProfile(string(data))
}

// parseProfile parses the profile text over the model defaults.
func parseProfile(data string) (Profile, error) {
	t, err := parseTOML(data)
	if err != nil {
		return Profile{}, fmt.Errorf("profile: %w", err)
	}
	spec := model.DefaultSpec()
	if err := decodeTOML(t, &spec); err != nil {
		return Profile{}, fmt.Errorf("profile: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return Profile{}, err
	}
	return Profile{
		Name:        spec.Name,
		Description: spec.Description,
		IdleRPM:     float32(spec.IdleRPM),
		RedlineRPM:  float32(spec.RedlineRPM),
		LimiterRPM:  float32(spec.LimiterRPM),
		spec:        &spec,
	}, nil
}

// New returns an engine for the profile at the given sample rate and tier.
// Tier2 renders the tier 1 model until tier 2 exists.
func New(p Profile, sampleRate int, tier Tier) (Engine, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate %d is not positive", sampleRate)
	}
	if tier > Tier2 {
		return nil, fmt.Errorf("tier %d is not known", tier)
	}
	if p.spec == nil {
		q, err := LoadProfile(p.Name)
		if err != nil {
			return nil, err
		}
		p = q
	}
	s, err := model.New(p.spec, sampleRate, tier >= Tier1)
	if err != nil {
		return nil, err
	}
	return &engine{synth: s}, nil
}

// LoadFilter derives Snapshot.Load from the car acceleration. The handling
// model has no engine load, so the game calls Update once per tick. The
// zero value is ready to use, with a time constant of 0.1 seconds.
type LoadFilter struct {
	// TimeConstant is the smoothing time in seconds. Zero selects 0.1.
	TimeConstant float32

	load float32
}

// Update returns the smoothed load, from 0 to 1. accel is the car
// acceleration in metres per second squared, peakAccel is the peak
// acceleration of the current gear, and dt is the tick length in seconds.
// The load is clamp(accel / peakAccel, 0, 1), and it is zero while the
// engine is on overrun.
func (f *LoadFilter) Update(accel, peakAccel, dt float32, overrun bool) float32 {
	var target float32
	if !overrun && peakAccel > 0 && accel > 0 {
		target = min(1, accel/peakAccel)
	}
	tc := f.TimeConstant
	if tc <= 0 {
		tc = 0.1
	}
	if dt > 0 {
		f.load += (target - f.load) * dt / (tc + dt)
	}
	return f.load
}

// engine is the Engine implementation. Set and Render meet at a sequence
// lock over three atomic words, so neither side allocates or blocks for
// long.
type engine struct {
	seq        atomic.Uint32
	w0, w1, w2 atomic.Uint64

	synth   *model.Synth
	last    Snapshot
	limiter uint8
	// primed is true after Render reads the first snapshot from Set.
	primed bool
}

func (e *engine) Set(s Snapshot) {
	for {
		q := e.seq.Load()
		if q&1 == 0 && e.seq.CompareAndSwap(q, q+1) {
			break
		}
		runtime.Gosched()
	}
	e.w0.Store(uint64(math.Float32bits(s.RPM)) | uint64(math.Float32bits(s.Throttle))<<32)
	e.w1.Store(uint64(math.Float32bits(s.Load)) | uint64(math.Float32bits(s.Speed))<<32)
	e.w2.Store(uint64(s.Gear) | uint64(s.Flags)<<8 | uint64(s.ShiftUp)<<16 |
		uint64(s.ShiftDown)<<24 | uint64(s.Limiter)<<32 | 1<<40)
	e.seq.Add(1)
}

// snapshot returns the latest consistent snapshot, and false before the
// first Set. When a Set is in progress for every try, it keeps the
// previous snapshot.
func (e *engine) snapshot() (Snapshot, bool) {
	for range 8 {
		q := e.seq.Load()
		if q&1 != 0 {
			continue
		}
		w0, w1, w2 := e.w0.Load(), e.w1.Load(), e.w2.Load()
		if e.seq.Load() != q {
			continue
		}
		if w2&(1<<40) == 0 { // No Set yet.
			return e.last, false
		}
		e.last = Snapshot{
			RPM:       math.Float32frombits(uint32(w0 & math.MaxUint32)),
			Throttle:  math.Float32frombits(uint32(w0 >> 32)),
			Load:      math.Float32frombits(uint32(w1 & math.MaxUint32)),
			Speed:     math.Float32frombits(uint32(w1 >> 32)),
			Gear:      uint8(w2 & 0xff),
			Flags:     Flags(w2 >> 8 & 0xff),
			ShiftUp:   uint8(w2 >> 16 & 0xff),
			ShiftDown: uint8(w2 >> 24 & 0xff),
			Limiter:   uint8(w2 >> 32 & 0xff),
		}
		return e.last, true
	}
	return e.last, e.primed
}

func (e *engine) Render(dst []float32) {
	s, ok := e.snapshot()
	events := 0
	if e.primed {
		events = int(s.Limiter - e.limiter)
	}
	e.limiter = s.Limiter
	e.primed = e.primed || ok
	e.synth.Render(dst, model.Control{
		RPM:           s.RPM,
		Throttle:      s.Throttle,
		Load:          s.Load,
		Running:       s.Flags&FlagRunning != 0,
		Overrun:       s.Flags&FlagOverrun != 0,
		LimiterEvents: events,
	})
}
