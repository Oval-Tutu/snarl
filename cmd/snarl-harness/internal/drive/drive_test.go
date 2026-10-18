package drive

import "testing"

const dt = 1.0 / 60

// run steps m for the given seconds at a fixed throttle.
func run(m *Model, throttle, seconds float64) {
	for range int(seconds / dt) {
		m.Step(throttle, true, dt)
	}
}

func TestRevsRiseWithThrottleAndFallOnLiftOff(t *testing.T) {
	m := New(DefaultGears)
	if m.Revs() != 0 {
		t.Fatalf("revs at rest %v, want 0", m.Revs())
	}
	run(m, 0.5, 0.5)
	up := m.Revs()
	if up <= 0 {
		t.Fatalf("revs %v after throttle, want above 0", up)
	}
	if m.Accel() <= 0 || m.PeakAccel() <= 0 {
		t.Errorf("accel %v, peak %v, want both above 0", m.Accel(), m.PeakAccel())
	}
	run(m, 0, 0.5)
	if m.Revs() >= up {
		t.Errorf("revs %v after lift-off, want below %v", m.Revs(), up)
	}
	run(m, 0, 30)
	if m.Revs() != 0 || m.Speed() != 0 {
		t.Errorf("revs %v, speed %v after a long coast, want idle at rest", m.Revs(), m.Speed())
	}
}

func TestOverrunOnLiftOff(t *testing.T) {
	m := New(DefaultGears)
	run(m, 0, 1)
	if m.Overrun() {
		t.Error("overrun at idle")
	}
	run(m, 0.8, 1)
	if m.Overrun() {
		t.Error("overrun with the throttle open")
	}
	m.Step(0, true, dt)
	if !m.Overrun() {
		t.Error("no overrun on lift-off")
	}
	m.Step(0, false, dt)
	if m.Overrun() {
		t.Error("overrun with the engine off")
	}
}

// TestLimiter checks that full throttle holds the revs at the limiter
// with no cut and no overrun, counts one arrival, and that a lift-off
// leaves the limiter.
func TestLimiter(t *testing.T) {
	m := New(DefaultGears)
	for i := 0; !m.Cut(); i++ {
		if i > 60*30 {
			t.Fatalf("not at the limiter after 30 s at full throttle, revs %v", m.Revs())
		}
		m.Step(1, true, dt)
	}
	if m.Limiter != 1 {
		t.Errorf("limiter count %d on arrival, want 1", m.Limiter)
	}
	for range 60 {
		m.Step(1, true, dt)
		if m.Revs() < 1-1e-6 || m.revs() > 1+1e-6 || !m.Cut() || m.Overrun() {
			t.Fatalf("at the limiter: revs %v, held %t, overrun %t", m.revs(), m.Cut(), m.Overrun())
		}
	}
	if m.Accel() != 0 || m.Limiter != 1 {
		t.Errorf("held at the limiter: accel %v, limiter count %d", m.Accel(), m.Limiter)
	}
	m.Step(0, true, dt)
	if m.Cut() || m.Revs() >= 1 || !m.Overrun() {
		t.Errorf("lift-off: revs %v, held %t, overrun %t", m.Revs(), m.Cut(), m.Overrun())
	}
	run(m, 0.5, 0.5)
	run(m, 1, 5)
	if m.Limiter != 2 || !m.Cut() {
		t.Errorf("second arrival: limiter count %d, held %t", m.Limiter, m.Cut())
	}
}

// TestDownshiftOverRev checks that revs past the limiter after a downshift
// fall back to the limiter and hold there with the throttle open.
func TestDownshiftOverRev(t *testing.T) {
	m := New(DefaultGears)
	m.Upshift()
	run(m, 1, 20)
	m.Downshift()
	if m.revs() <= 1 {
		t.Fatalf("revs %v after the downshift, want past the limiter", m.revs())
	}
	run(m, 1, 5)
	if m.revs() > 1+1e-6 || !m.Cut() {
		t.Errorf("revs %v, held %t after the over-rev", m.revs(), m.Cut())
	}
}

func TestShiftsMoveRevs(t *testing.T) {
	m := New(DefaultGears)
	run(m, 0.7, 2)
	before := m.Revs()
	if !m.Upshift() || m.Gear() != 1 || m.ShiftUp != 1 {
		t.Fatalf("upshift: gear %d, count %d", m.Gear(), m.ShiftUp)
	}
	if m.Revs() >= before {
		t.Errorf("revs %v after upshift, want below %v", m.Revs(), before)
	}
	up := m.Revs()
	if !m.Downshift() || m.Gear() != 0 || m.ShiftDown != 1 {
		t.Fatalf("downshift: gear %d, count %d", m.Gear(), m.ShiftDown)
	}
	if m.Revs() <= up {
		t.Errorf("revs %v after downshift, want above %v", m.Revs(), up)
	}
	if m.Downshift() || m.ShiftDown != 1 {
		t.Error("downshift below first gear")
	}
}

func TestGearCounts(t *testing.T) {
	for n := MinGears; n <= MaxGears; n++ {
		m := New(n)
		if m.Gears() != n {
			t.Fatalf("gears %d, want %d", m.Gears(), n)
		}
		prev := 0.0
		for g := range n {
			if r := m.ratio(g); r <= prev {
				t.Errorf("%d gears: gear %d top speed %v, not above %v", n, g, r, prev)
			}
			prev = m.ratio(g)
		}
		if top := m.ratio(n - 1); top != topSpeed {
			t.Errorf("%d gears: top gear top speed %v, want %v", n, top, topSpeed)
		}
		for range n + 2 {
			run(m, 1, 1)
			m.Upshift()
		}
		if m.Gear() != n-1 || int(m.ShiftUp) != n-1 {
			t.Errorf("%d gears: gear %d after upshifts, %d shifts", n, m.Gear(), m.ShiftUp)
		}
		if m.Revs() <= 0 || m.Speed() <= 0 {
			t.Errorf("%d gears: revs %v, speed %v in top gear", n, m.Revs(), m.Speed())
		}
	}
	m := New(MaxGears)
	m.Upshift()
	m.Upshift()
	m.Upshift()
	m.SetGears(1)
	if m.Gears() != MinGears || m.Gear() != MinGears-1 {
		t.Errorf("SetGears(1): %d gears, gear %d", m.Gears(), m.Gear())
	}
	m.SetGears(9)
	if m.Gears() != MaxGears {
		t.Errorf("SetGears(9): %d gears", m.Gears())
	}
}
