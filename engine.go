package snarl

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"math"
	"path"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/oval-tutu/snarl/internal/core"
	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/esynth"
)

// The embedded profiles are the reference set of profiles/README.md, with
// the cycle tables and the impulse responses that they name. The sources
// files beside them are not embedded.
//
//go:embed profiles/i4-vtec.toml profiles/v8-ls.toml profiles/f4-ej25.toml
//go:embed profiles/v12-412.toml profiles/v12-merlin.toml
//go:embed profiles/cycles/*.bin profiles/ir/*.wav
var profileFiles embed.FS

// Profile is a parsed engine profile.
type Profile struct {
	// Name is the profile identifier, for example "v8-ls".
	Name string
	// Description names the engine type.
	Description string
	// IdleRPM, RedlineRPM and LimiterRPM are the engine speeds of the
	// profile, in revolutions per minute.
	IdleRPM, RedlineRPM, LimiterRPM float32

	spec *core.Spec
}

// ErrUnknownProfile is returned when no embedded profile has the name.
var ErrUnknownProfile = errors.New("unknown engine profile")

// ErrNoTables is returned for a profile that names no cycle tables, for
// example a catalogue profile in the format before the cycle tables.
var ErrNoTables = errors.New("profile has no cycle tables")

// Profiles returns the names of the embedded engine profiles: five
// engine-sim engines, each converted from its upstream engine
// (profiles/README.md).
func Profiles() []string {
	return []string{"i4-vtec", "v8-ls", "f4-ej25", "v12-412", "v12-merlin"}
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

// parseProfile parses the profile text over the core defaults, and checks
// that the embedded tables and impulse responses that it names exist and
// match.
func parseProfile(data string) (Profile, error) {
	t, err := parseTOML(data)
	if err != nil {
		return Profile{}, fmt.Errorf("profile: %w", err)
	}
	if _, ok := t.values["tables"]; !ok {
		name := t.values["name"].str
		return Profile{}, fmt.Errorf("profile %q: %w", name, ErrNoTables)
	}
	spec := core.DefaultSpec()
	if err := decodeTOML(t, &spec); err != nil {
		return Profile{}, fmt.Errorf("profile: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return Profile{}, err
	}
	if _, err := resources(&spec); err != nil {
		return Profile{}, fmt.Errorf("profile %s: %w", spec.Name, err)
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

// resource is one embedded file, decoded once for the life of the
// process and shared by every engine.
type resource struct {
	once sync.Once
	v    any
	err  error
}

var resourceCache sync.Map

// cached returns the decoded value of the embedded file at name.
func cached(name string, decode func([]byte) (any, error)) (any, error) {
	v, _ := resourceCache.LoadOrStore(name, &resource{})
	r := v.(*resource)
	r.once.Do(func() {
		data, err := profileFiles.ReadFile(name)
		if err != nil {
			r.err = err
			return
		}
		r.v, r.err = decode(data)
	})
	return r.v, r.err
}

// resources returns the cycle tables and the impulse responses of the
// spec. The impulse response of each channel of the profile must be the
// one that the tables name.
func resources(s *core.Spec) (core.Resources, error) {
	v, err := cached("profiles/cycles/"+s.Tables+".bin", func(b []byte) (any, error) {
		t, err := cycles.Read(b)
		if err != nil {
			return nil, err
		}
		return cycles.NewBank(t)
	})
	if err != nil {
		return core.Resources{}, fmt.Errorf("cycle tables %q: %w", s.Tables, err)
	}
	bank := v.(*cycles.Bank)
	t := bank.Tables()
	if len(t.Channels) != len(s.Channels) {
		return core.Resources{}, fmt.Errorf("%d channels, the tables %q have %d", len(s.Channels), s.Tables, len(t.Channels))
	}
	res := core.Resources{Bank: bank}
	for i, c := range s.Channels {
		if want := path.Base(t.Channels[i].ImpulseResponse); c.ImpulseResponse != want {
			return core.Resources{}, fmt.Errorf("channel %d: impulse response %q, the tables name %q", i+1, c.ImpulseResponse, want)
		}
		name := "profiles/ir/" + c.ImpulseResponse
		v, err := cached(name, func(b []byte) (any, error) {
			return esynth.DecodeImpulseResponse(bytes.NewReader(b), name)
		})
		if err != nil {
			return core.Resources{}, err
		}
		res.ImpulseResponses = append(res.ImpulseResponses, v.([]int16))
	}
	return res, nil
}

// New returns an engine for the profile at the given sample rate and tier.
// Both tiers render the same engine. Any other tier is an error.
func New(p Profile, sampleRate int, tier Tier) (Engine, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate %d is not positive", sampleRate)
	}
	if tier != Tier1 && tier != Tier2 {
		return nil, fmt.Errorf("tier %d is not known", tier)
	}
	if p.spec == nil {
		q, err := LoadProfile(p.Name)
		if err != nil {
			return nil, err
		}
		p = q
	}
	res, err := resources(p.spec)
	if err != nil {
		return nil, err
	}
	s, err := core.New(p.spec, res, sampleRate)
	if err != nil {
		return nil, err
	}
	return &engine{synth: s}, nil
}

// LoadFilter derives Snapshot.Load from the throttle and the car
// acceleration. The handling model has no engine load, so the game calls
// Update once per tick. The zero value is ready to use, with a time
// constant of 0.1 seconds.
type LoadFilter struct {
	// TimeConstant is the smoothing time in seconds. Zero selects 0.1.
	TimeConstant float32

	load float32
}

// Update returns the smoothed load, from 0 to 1. throttle is the throttle
// position from 0 to 1, accel is the car acceleration in metres per second
// squared, peakAccel is the peak acceleration of the current gear, and dt
// is the tick length in seconds. The load is the throttle, raised to
// accel / peakAccel when that is higher, and clamped from 0 to 1. A steady
// part throttle, with no acceleration, keeps its load. The load is zero
// while the engine is on overrun.
func (f *LoadFilter) Update(throttle, accel, peakAccel, dt float32, overrun bool) float32 {
	var target float32
	if !overrun {
		target = min(1, max(0, throttle))
		if peakAccel > 0 && accel > 0 {
			target = max(target, min(1, accel/peakAccel))
		}
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
// lock over four atomic words, so neither side allocates or blocks for
// long. Render publishes the pop report in one atomic word, so that Pops
// never waits.
type engine struct {
	seq            atomic.Uint32
	w0, w1, w2, w3 atomic.Uint64
	report         atomic.Uint64

	synth    *core.Engine
	last     Snapshot
	limiter  uint8
	popReq   uint8
	reported uint32
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
	e.w3.Store(uint64(math.Float32bits(s.PopStrength)) | uint64(s.PopRequest)<<32 | uint64(s.PopCharacter)<<40)
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
		w0, w1, w2, w3 := e.w0.Load(), e.w1.Load(), e.w2.Load(), e.w3.Load()
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

			PopStrength:  math.Float32frombits(uint32(w3 & math.MaxUint32)),
			PopRequest:   uint8(w3 >> 32 & 0xff),
			PopCharacter: PopCharacter(w3 >> 40 & 0xff),
		}
		return e.last, true
	}
	return e.last, e.primed
}

func (e *engine) Render(dst []float32) {
	s, ok := e.snapshot()
	events, requests := 0, 0
	if e.primed {
		events = int(s.Limiter - e.limiter)
		requests = int(s.PopRequest - e.popReq)
	}
	e.limiter, e.popReq = s.Limiter, s.PopRequest
	e.primed = e.primed || ok
	e.synth.Render(dst, core.Control{
		RPM:           s.RPM,
		Throttle:      s.Throttle,
		Load:          s.Load,
		Running:       s.Flags&FlagRunning != 0,
		Overrun:       s.Flags&FlagOverrun != 0,
		Final:         s.Flags&FlagFinalGear != 0,
		LimiterEvents: events,
		Gear:          s.Gear,
		PopRequests:   requests,
		PopStrength:   s.PopStrength,
		NoPops:        s.Flags&FlagNoPops != 0,
		Character:     int(s.PopCharacter),
	})
	n, kind, src, strength := e.synth.PopReport()
	if n != e.reported {
		e.reported = n
		q := uint64(min(max(strength, 0), 1)*math.MaxUint16 + 0.5)
		k, c := uint64(kind+1)&0xff, uint64(src)&0xff //nolint:gosec // The kinds and sources are below 8.
		e.report.Store(uint64(n) | k<<32 | c<<40 | q<<48)
	}
}

// Pops returns the pop report. The core kinds count from 0 in the order of
// PopKind from KindPop, and the core sources have the PopSource values.
func (e *engine) Pops() PopReport {
	v := e.report.Load()
	return PopReport{
		Count:    uint32(v & math.MaxUint32),
		Kind:     PopKind(v >> 32 & 0xff),
		Source:   PopSource(v >> 40 & 0xff),
		Strength: float32(v>>48) / math.MaxUint16,
	}
}
