package snarl

// Tier selects the cost and detail of the synthesis model. See
// docs/scoping.md section 3.
type Tier uint8

// Quality tiers. The values start at 1.
const (
	// Tier1 is for all targets, from the Nintendo Switch to modern PCs.
	Tier1 Tier = iota + 1
	// Tier2 is for modern PCs.
	Tier2
)

// Flags hold the engine and car states that change the sound.
type Flags uint8

// Snapshot flags.
const (
	// FlagRunning is set while the engine runs. When it is clear, the
	// engine is silent.
	FlagRunning Flags = 1 << iota
	// FlagAirborne is set while the driven wheels are off the ground.
	FlagAirborne
	// FlagWheelspin is set while the driven wheels spin.
	FlagWheelspin
	// FlagClutch is set while the clutch is disengaged.
	FlagClutch
	// FlagRedZone is set while the RPM is in the red zone.
	FlagRedZone
	// FlagOverrun is set while the throttle is closed and the wheels
	// drive the engine.
	FlagOverrun
	// FlagFinalGear is set while the selected gear is the last gear. In
	// the last gear, the engine holds a steady tone at the limiter RPM and
	// does not bounce off the limiter.
	FlagFinalGear
	// FlagNoPops is set while the engine must make no pops of its own on
	// a lift-off or a coast. Requested pops and ignition cut bangs still
	// play.
	FlagNoPops
)

// PopCharacter selects the character of the exhaust pops.
type PopCharacter uint8

// Pop characters. A car upgrade in the game can select one at run time.
const (
	// CharacterProfile is the character of the profile.
	CharacterProfile PopCharacter = iota
	// CharacterStock is a standard exhaust: fewer and softer own pops,
	// with more pops and burbles.
	CharacterStock
	// CharacterSport is a factory sport exhaust, the default.
	CharacterSport
	// CharacterRace is a free-flowing race exhaust: more and louder own
	// pops, with more booms and crackles.
	CharacterRace
)

// PopKind is the kind of an exhaust pop.
type PopKind uint8

// Pop kinds.
const (
	// KindNone is the kind of the report before the first pop.
	KindNone PopKind = iota
	// KindPop is an isolated pop with a resonant body.
	KindPop
	// KindBoom is a short deep burst with a bright onset.
	KindBoom
	// KindCrackle is a train of cracks over a deep body.
	KindCrackle
	// KindBurble is a soft low pop with no crack.
	KindBurble
)

// String returns the name of the kind.
func (k PopKind) String() string {
	switch k {
	case KindPop:
		return "pop"
	case KindBoom:
		return "boom"
	case KindCrackle:
		return "crackle"
	case KindBurble:
		return "burble"
	}
	return "none"
}

// PopSource is the cause of an exhaust pop.
type PopSource uint8

// Pop sources.
const (
	// SourceNone is the source of the report before the first pop.
	SourceNone PopSource = iota
	// SourceRequest is a pop that the game requests with PopRequest.
	SourceRequest
	// SourceOwn is a pop that the engine makes on a lift-off or a coast.
	SourceOwn
	// SourceLimiter is the bang of an ignition cut at the rev limiter.
	SourceLimiter
)

// String returns the name of the source.
func (s PopSource) String() string {
	switch s {
	case SourceRequest:
		return "request"
	case SourceOwn:
		return "own"
	case SourceLimiter:
		return "limiter"
	}
	return "none"
}

// PopReport is the pop state that the engine reports to the game.
type PopReport struct {
	// Count is the number of pops that started since New. It wraps
	// around after 2^32 pops.
	Count uint32
	// Kind, Source and Strength describe the latest pop. Strength is
	// the requested strength for a requested pop. For another pop, it is
	// the random level of the pop over the highest level, from 0.35 to 1.
	Kind     PopKind
	Source   PopSource
	Strength float32
}

// Snapshot is the engine state that the game writes once per tick. The
// engine reads the latest snapshot once per audio block.
type Snapshot struct {
	// RPM is the engine speed in revolutions per minute.
	RPM float32
	// Throttle is the throttle position, from 0 to 1.
	Throttle float32
	// Load is the engine load, from 0 to 1. The game can calculate it
	// from the car acceleration. Offline renders set it directly.
	Load float32
	// Speed is the car speed in metres per second.
	Speed float32
	// Gear is the selected gear. 0 is the lowest gear. The number of
	// gears is not fixed.
	Gear uint8
	// Flags hold the engine and car states.
	Flags Flags
	// ShiftUp, ShiftDown and Limiter are event counters. The game
	// increments a counter for each event. The engine detects an event
	// when the counter changes, so no event is lost between two blocks.
	ShiftUp, ShiftDown, Limiter uint8
	// PopRequest is a counter of pop requests. The game increments it for
	// each pop that it wants, for example on a gear change under the
	// right conditions. Each increment plays one pop at the next audio
	// block, with no random chance and no wall heat, at least 50 ms after
	// the pop before. A request inside those 50 ms waits. Up to four
	// requests wait, and a later request in excess of that is lost.
	PopRequest uint8
	// PopStrength is the strength of the latest request, from 0 to 1. It
	// picks the pop size: below 1/3 a pop or a burble, below 2/3 a pop or
	// a boom, and from 2/3 a boom or a short crackle.
	PopStrength float32
	// PopCharacter selects the pop character. CharacterProfile, or a
	// value above CharacterRace, keeps the character of the profile.
	PopCharacter PopCharacter
}

// Engine makes the sound of one engine.
type Engine interface {
	// Set stores the latest snapshot. The game goroutine calls Set. It
	// is safe to call Set while Render runs on another goroutine.
	Set(s Snapshot)
	// Render fills dst with mono samples in the range -1 to 1. The
	// audio goroutine calls Render. Render does not allocate memory.
	Render(dst []float32)
	// Pops returns the pop report as of the end of the latest Render.
	// The game goroutine can call it at any time. It does not block
	// Render.
	Pops() PopReport
}
