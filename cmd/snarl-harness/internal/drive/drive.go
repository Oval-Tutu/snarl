// Package drive is a simple drivetrain that drives the harness engine. It
// follows the rev model of FiveHorizons internal/handling/handling.go
// (Model.engine and revClimb) for any number of gears, without the brake,
// the shift window, the gear ratio slide, the wheelspin, and the road.
package drive

import "math"

// The range and the default of the number of gears.
const (
	MinGears     = 2
	MaxGears     = 6
	DefaultGears = 3
)

// Speeds in metres per second.
const (
	kmh = 1 / 3.6

	topSpeed     = 290 * kmh // Full throttle top speed in top gear, HighTop in FiveHorizons.
	firstTop     = 190 * kmh // Full throttle top speed in first gear with two gears, LowTop in FiveHorizons.
	firstTopStep = 25 * kmh  // Each gear past two lowers the first gear top speed by this.
	launchSpeed  = 40 * kmh  // Below this speed the climb rate doubles.
)

// Rev model tuning. Revs run from 0 at idle to 1 at the limiter.
const (
	climbUnit    = 0.0024202 // Rev climb per second in top gear for each step of revClimb.
	climbSteps   = 249.29    // The revClimb index at full revs.
	revClimbPeak = 0x30      // The largest step of revClimb.
	decay        = 0.2       // Rev fall per second with the throttle lifted.
	overRev      = 2 * decay // Rev fall above full revs, after a downshift at speed.
	maxAccel     = 80.0      // Largest speed change in m/s per second.

	// throttleReach is the goal in revs at full throttle. It is past 1, so
	// that full throttle reaches the limiter.
	throttleReach = 1.05
	// CutTime is the time in seconds for which the limiter cuts the throttle.
	CutTime = 0.08
	// OverrunThrottle is the throttle below which falling revs are overrun.
	OverrunThrottle = 0.05
)

// revClimb is the rev climb rate against the revs, from 0 at idle to
// climbSteps at full revs, in steps of climbUnit: the table of the
// original. It is soft at idle, strongest at about 75 percent, and it
// tails off near the top.
var revClimb = [256]uint8{
	0x14, 0x14, 0x14, 0x14, 0x15, 0x15, 0x15, 0x15, 0x16, 0x16, 0x16, 0x16, 0x17, 0x17, 0x17, 0x17,
	0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18,
	0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19,
	0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19, 0x19,
	0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A,
	0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A, 0x1A,
	0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B, 0x1B,
	0x1C, 0x1C, 0x1C, 0x1C, 0x1C, 0x1C, 0x1C, 0x1C, 0x1D, 0x1D, 0x1D, 0x1D, 0x1D, 0x1D, 0x1D, 0x1D,
	0x1E, 0x1E, 0x1E, 0x1E, 0x1E, 0x1E, 0x1E, 0x1E, 0x1F, 0x1F, 0x1F, 0x1F, 0x20, 0x20, 0x20, 0x20,
	0x21, 0x21, 0x22, 0x22, 0x23, 0x23, 0x24, 0x24, 0x25, 0x25, 0x26, 0x26, 0x27, 0x27, 0x28, 0x28,
	0x29, 0x29, 0x2A, 0x2A, 0x2B, 0x2B, 0x2B, 0x2C, 0x2C, 0x2C, 0x2D, 0x2D, 0x2D, 0x2E, 0x2E, 0x2E,
	0x2F, 0x2F, 0x2F, 0x2F, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x2F, 0x2F, 0x2F, 0x2F,
	0x2E, 0x2E, 0x2E, 0x2D, 0x2D, 0x2D, 0x2C, 0x2C, 0x2B, 0x2B, 0x2A, 0x2A, 0x29, 0x29, 0x28, 0x28,
	0x27, 0x26, 0x25, 0x24, 0x23, 0x22, 0x21, 0x20, 0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18,
	0x17, 0x15, 0x13, 0x11, 0x0F, 0x0D, 0x0B, 0x0A, 0x09, 0x09, 0x08, 0x08, 0x07, 0x07, 0x06, 0x06,
	0x05, 0x05, 0x05, 0x04, 0x04, 0x04, 0x03, 0x03, 0x03, 0x02, 0x02, 0x02, 0x01, 0x01, 0x01, 0x01,
}

// Model is the drivetrain state. The car speed is the state, and the revs
// are the speed over the ratio of the selected gear, so a change of gear
// keeps the speed and moves the revs by the ratio step. The zero value is
// not ready to use: call New.
type Model struct {
	gears, gear int
	speed       float64 // Car speed in m/s.
	accel       float64 // Car acceleration of the last step, m/s per second.
	cut         float64 // Seconds left of the limiter cut.
	overrun     bool

	// ShiftUp, ShiftDown and Limiter count the events, as in
	// snarl.Snapshot.
	ShiftUp, ShiftDown, Limiter uint8
}

// New returns a car at rest in first gear with n gears, see SetGears.
func New(n int) *Model {
	m := &Model{}
	m.SetGears(n)
	return m
}

// Gears returns the number of gears.
func (m *Model) Gears() int { return m.gears }

// Gear returns the selected gear. 0 is first gear.
func (m *Model) Gear() int { return m.gear }

// SetGears sets the number of gears, clamped to MinGears and MaxGears. The
// selected gear stays, or becomes the top gear when it no longer exists.
func (m *Model) SetGears(n int) {
	m.gears = min(max(n, MinGears), MaxGears)
	m.gear = min(m.gear, m.gears-1)
}

// Upshift selects the next gear up, and reports false in top gear.
func (m *Model) Upshift() bool {
	if m.gear >= m.gears-1 {
		return false
	}
	m.gear++
	m.ShiftUp++
	return true
}

// Downshift selects the next gear down, and reports false in first gear.
func (m *Model) Downshift() bool {
	if m.gear == 0 {
		return false
	}
	m.gear--
	m.ShiftDown++
	return true
}

// ratio returns the full throttle top speed of gear g. The top speeds run
// in a geometric series from first gear to topSpeed.
func (m *Model) ratio(g int) float64 {
	first := firstTop - firstTopStep*float64(m.gears-MinGears)
	return first * math.Pow(topSpeed/first, float64(g)/float64(m.gears-1))
}

// revs returns the engine revs, 0 at idle and 1 at the limiter. They can
// pass 1 after a downshift at speed.
func (m *Model) revs() float64 { return m.speed / m.ratio(m.gear) }

// Revs returns the engine revs, from 0 at idle to 1 at the limiter.
func (m *Model) Revs() float64 { return min(m.revs(), 1) }

// RPM returns the engine speed for an engine with the given idle and
// limiter RPM.
func (m *Model) RPM(idle, limiter float32) float32 {
	return idle + float32(m.Revs())*(limiter-idle)
}

// Speed returns the car speed in m/s.
func (m *Model) Speed() float64 { return m.speed }

// Accel returns the car acceleration of the last step in m/s per second.
func (m *Model) Accel() float64 { return m.accel }

// PeakAccel returns the largest car acceleration of the selected gear at
// the current speed, for snarl.LoadFilter.
func (m *Model) PeakAccel() float64 {
	return m.climbRate(revClimbPeak) * m.ratio(m.gear)
}

// Overrun reports whether the throttle was closed and the revs fell or
// were above idle in the last step.
func (m *Model) Overrun() bool { return m.overrun }

// Cut reports whether the limiter cuts the throttle.
func (m *Model) Cut() bool { return m.cut > 0 }

// climbRate returns the rev climb rate per second in the selected gear for
// the revClimb step c. A lower gear climbs faster by the square of its
// ratio step from top gear.
func (m *Model) climbRate(c float64) float64 {
	g := topSpeed / m.ratio(m.gear)
	rate := climbUnit * c * g * g
	if m.speed < launchSpeed {
		rate *= 2
	}
	return rate
}

// Step advances the model by dt seconds with the throttle from 0 to 1.
// While the engine is not running, the throttle does nothing.
func (m *Model) Step(throttle float64, running bool, dt float64) {
	if dt <= 0 {
		return
	}
	ratio := m.ratio(m.gear)
	revs := m.revs()
	goal := throttle * throttleReach
	if !running {
		goal = 0
	}
	if m.cut > 0 {
		m.cut -= dt
		goal = 0
	}
	var next float64
	if goal > revs {
		i := min(max(int(revs*climbSteps), 0), len(revClimb)-1)
		next = min(revs+m.climbRate(float64(revClimb[i]))*dt, goal)
	} else {
		fall := decay
		if revs > 1 {
			fall = overRev
		}
		next = max(revs-fall*dt, goal)
	}
	v := m.speed + min(max(next*ratio-m.speed, -maxAccel*dt), maxAccel*dt)
	v = max(v, 0)
	m.accel = (v - m.speed) / dt
	m.speed = v
	now := m.revs()
	if now >= 1 && goal >= 1 && m.cut <= 0 {
		m.Limiter++
		m.cut = CutTime
	}
	m.overrun = running && throttle < OverrunThrottle && (now < revs || now > 0)
}
