// The fake gamepads and the keyd tests are copies from FiveHorizons
// internal/input/input_test.go, and padA and padThrottle are copies from
// internal/input/input.go.
package pads

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	padA        = ebiten.StandardGamepadButtonRightBottom
	padThrottle = ebiten.StandardGamepadButtonFrontBottomRight
)

// fakePads is a set of gamepads for the tests of pads.
type fakePads struct {
	ids   []ebiten.GamepadID
	std   map[ebiten.GamepadID]bool
	busy  map[ebiten.GamepadID]bool
	names map[ebiten.GamepadID][2]string // The SDL GUID and the name.
	sent  []vibration                    // The rumble that the pads received.
	down  map[padButton]bool             // The buttons that are down.
	just  map[padButton]bool             // The buttons that went down in this frame.
	vals  map[padButton]float64          // The analogue buttons.
	axes  map[padAxis]float64
}

// padButton is one button of one fake gamepad.
type padButton struct {
	id ebiten.GamepadID
	b  ebiten.StandardGamepadButton
}

// padAxis is one axis of one fake gamepad.
type padAxis struct {
	id ebiten.GamepadID
	a  ebiten.StandardGamepadAxis
}

// vibration is one call of VibrateGamepad.
type vibration struct {
	id ebiten.GamepadID
	o  ebiten.VibrateGamepadOptions
}

func (f *fakePads) appendIDs(ids []ebiten.GamepadID) []ebiten.GamepadID {
	return append(ids, f.ids...)
}
func (f *fakePads) standard(id ebiten.GamepadID) bool { return f.std[id] }
func (f *fakePads) used(id ebiten.GamepadID) bool     { return f.busy[id] }
func (f *fakePads) identity(id ebiten.GamepadID) (guid, name string) {
	n := f.names[id]
	return n[0], n[1]
}

func (f *fakePads) vibrate(id ebiten.GamepadID, o *ebiten.VibrateGamepadOptions) {
	f.sent = append(f.sent, vibration{id, *o})
}

func (f *fakePads) pressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool {
	return f.down[padButton{id, b}]
}

func (f *fakePads) justPressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool {
	return f.just[padButton{id, b}]
}

func (f *fakePads) value(id ebiten.GamepadID, b ebiten.StandardGamepadButton) float64 {
	return f.vals[padButton{id, b}]
}

func (f *fakePads) axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64 {
	return f.axes[padAxis{id, a}]
}

// hold puts button b of gamepad id down, and press marks it as down in
// this frame as well.
func (f *fakePads) hold(id ebiten.GamepadID, b ebiten.StandardGamepadButton) {
	f.down[padButton{id, b}] = true
}

func (f *fakePads) press(id ebiten.GamepadID, b ebiten.StandardGamepadButton) {
	f.hold(id, b)
	f.just[padButton{id, b}] = true
}

func newFake() *fakePads {
	return &fakePads{
		std: map[ebiten.GamepadID]bool{}, busy: map[ebiten.GamepadID]bool{}, names: map[ebiten.GamepadID][2]string{},
		down: map[padButton]bool{}, just: map[padButton]bool{}, vals: map[padButton]float64{}, axes: map[padAxis]float64{},
	}
}

func (f *fakePads) plug(id ebiten.GamepadID, std bool) {
	f.ids = append(f.ids, id)
	f.std[id] = std
}

func wantPad(t *testing.T, p *pads, id ebiten.GamepadID, ok bool) {
	t.Helper()
	if p.drive.ok != ok || ok && p.drive.id != id {
		t.Fatalf("active gamepad %d, %t, want %d, %t", p.drive.id, p.drive.ok, id, ok)
	}
}

// The keyd pointer at ID 0 has no standard layout, so the controller at
// ID 1 is the active gamepad.
func TestPadsSkipKeydPointer(t *testing.T) {
	f := newFake()
	f.plug(0, false) // The virtual pointer of keyd.
	f.plug(1, true)
	p := newPads(f)
	p.update()
	wantPad(t, &p, 1, true)
	f.busy[0] = true
	p.update()
	wantPad(t, &p, 1, true)
}

func TestPadsNoStandardGamepad(t *testing.T) {
	f := newFake()
	p := newPads(f)
	p.update()
	wantPad(t, &p, 0, false)
	f.plug(0, false)
	f.busy[0] = true
	p.update()
	wantPad(t, &p, 0, false)
	f.press(0, padA)
	f.vals[padButton{0, padThrottle}] = 1
	if p.pressed(p.drive, padA) || p.value(p.drive, padThrottle) != 0 {
		t.Error("no gamepad reads as pressed")
	}
}
