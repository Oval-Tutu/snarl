package snarl

// Tier selects the cost and detail of the synthesis model. See
// docs/scoping.md section 3.
type Tier uint8

// Quality tiers, from the cheapest to the most detailed.
const (
	// Tier0 is for the Nintendo Switch and old phones.
	Tier0 Tier = iota
	// Tier1 is for mid-range PCs and phones.
	Tier1
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
)

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
}

// Engine makes the sound of one engine.
type Engine interface {
	// Set stores the latest snapshot. The game goroutine calls Set. It
	// is safe to call Set while Render runs on another goroutine.
	Set(s Snapshot)
	// Render fills dst with mono samples in the range -1 to 1. The
	// audio goroutine calls Render. Render does not allocate memory.
	Render(dst []float32)
}
