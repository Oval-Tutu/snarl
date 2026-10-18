// Package pads reads the gamepad that the player uses. It is a copy of
// FiveHorizons internal/input/pads.go, with StickDead from
// internal/input/input.go. The changes make it stand alone: Reader holds
// the pads and the harness reads the driver gamepad through it, and the
// stand-in pads for screenshots and the unused stick readers are gone.
package pads

import (
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// StickDead is the radius of the dead zone of the left stick.
const StickDead = 0.13

// Reader reads the gamepad that the player uses, see pads.
type Reader struct {
	pads pads
}

// NewReader returns a Reader of the Ebitengine gamepads.
func NewReader() *Reader { return &Reader{pads: newPads(ebitenPads{})} }

// Poll follows the connected gamepads. Call it once a frame. It reports
// whether the player switched to another gamepad.
func (r *Reader) Poll() bool { return r.pads.update() }

// Pressed reports whether button b of the driver gamepad is down.
func (r *Reader) Pressed(b ebiten.StandardGamepadButton) bool {
	return r.pads.pressed(r.pads.drive, b)
}

// JustPressed reports whether button b of the driver gamepad went down in
// this frame.
func (r *Reader) JustPressed(b ebiten.StandardGamepadButton) bool {
	return r.pads.justPressed(r.pads.drive, b)
}

// Value returns how far button b of the driver gamepad is down, from 0 to 1.
func (r *Reader) Value(b ebiten.StandardGamepadButton) float64 {
	return r.pads.value(r.pads.drive, b)
}

// Name returns the name of the driver gamepad, and false when no gamepad
// with the standard layout is connected.
func (r *Reader) Name() (string, bool) {
	d := r.pads.drive
	if !d.ok {
		return "", false
	}
	for _, k := range r.pads.known {
		if k.ID == d.id {
			return k.Name, true
		}
	}
	return "", true
}

// padSource reads the connected gamepads and their buttons and axes.
// ebitenPads reads Ebitengine, and the tests give a fake.
type padSource interface {
	appendIDs(ids []ebiten.GamepadID) []ebiten.GamepadID
	standard(id ebiten.GamepadID) bool
	used(id ebiten.GamepadID) bool // A button is down, or a stick is past the dead zone.
	identity(id ebiten.GamepadID) (guid, name string)
	vibrate(id ebiten.GamepadID, o *ebiten.VibrateGamepadOptions)
	pressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool
	justPressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool // The button went down in this frame.
	value(id ebiten.GamepadID, b ebiten.StandardGamepadButton) float64
	axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64
}

type ebitenPads struct{}

func (ebitenPads) appendIDs(ids []ebiten.GamepadID) []ebiten.GamepadID {
	return ebiten.AppendGamepadIDs(ids)
}

func (ebitenPads) standard(id ebiten.GamepadID) bool {
	return ebiten.IsStandardGamepadLayoutAvailable(id)
}

func (ebitenPads) used(id ebiten.GamepadID) bool {
	for b := ebiten.StandardGamepadButton(0); b <= ebiten.StandardGamepadButtonMax; b++ {
		if ebiten.IsStandardGamepadButtonPressed(id, b) {
			return true
		}
	}
	for a := ebiten.StandardGamepadAxisLeftStickHorizontal; a < ebiten.StandardGamepadAxisMax; a += 2 {
		if math.Hypot(ebiten.StandardGamepadAxisValue(id, a), ebiten.StandardGamepadAxisValue(id, a+1)) > StickDead {
			return true
		}
	}
	return false
}

func (ebitenPads) identity(id ebiten.GamepadID) (guid, name string) {
	return ebiten.GamepadSDLID(id), ebiten.GamepadName(id)
}

func (ebitenPads) vibrate(id ebiten.GamepadID, o *ebiten.VibrateGamepadOptions) {
	ebiten.VibrateGamepad(id, o)
}

func (ebitenPads) pressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool {
	return ebiten.IsStandardGamepadButtonPressed(id, b)
}

func (ebitenPads) justPressed(id ebiten.GamepadID, b ebiten.StandardGamepadButton) bool {
	return inpututil.IsStandardGamepadButtonJustPressed(id, b)
}

func (ebitenPads) value(id ebiten.GamepadID, b ebiten.StandardGamepadButton) float64 {
	return ebiten.StandardGamepadButtonValue(id, b)
}

func (ebitenPads) axis(id ebiten.GamepadID, a ebiten.StandardGamepadAxis) float64 {
	return ebiten.StandardGamepadAxisValue(id, a)
}

// Pad is a connected gamepad with the standard layout. The GUID, the name,
// and the index identify it from one run to the next, where the ID can
// change.
type Pad struct {
	ID    ebiten.GamepadID
	GUID  string // The SDL GUID, see ebiten.GamepadSDLID, or empty where the platform has none.
	Name  string
	Index int // The place among the connected gamepads with the same GUID and name, in order of ID.
}

// Match returns the index in pads of the gamepad with guid and name at
// place index among the gamepads with both, or the first of them when fewer
// are connected, and false when none is connected.
func Match(pads []Pad, guid, name string, index int) (int, bool) {
	at, ok := 0, false
	for i, p := range pads {
		if p.GUID != guid || p.Name != name {
			continue
		}
		if p.Index == index {
			return i, true
		}
		if !ok {
			at, ok = i, true
		}
	}
	return at, ok
}

// pad is one gamepad that a reader reads, or none.
type pad struct {
	id ebiten.GamepadID
	ok bool // id holds a connected gamepad.
}

// pads picks the gamepads that the player uses. The menus read the gamepad
// with the standard layout used last, or the first one until the player
// uses another. The driver controls read the chosen gamepad while it is
// connected, see Reader.Prefer, and otherwise the gamepad used last. A
// device without the standard layout, such as the virtual pointer of the
// keyd key remapper, has no known A button or left stick, so pads never
// reads it.
type pads struct {
	src   padSource
	ids   []ebiten.GamepadID // The buffer of the connected gamepads.
	prev  []ebiten.GamepadID // The connected gamepads when known was built.
	known []Pad              // The connected gamepads with the standard layout.
	gen   int                // Counts the changes of known.
	menu  pad                // The gamepad used last.
	drive pad                // The gamepad of the driver controls.

	want     bool // A gamepad is chosen: wantGUID, wantName, and wantIdx identify it.
	wantGUID string
	wantName string
	wantIdx  int
}

func newPads(src padSource) pads {
	return pads{
		src:   src,
		ids:   make([]ebiten.GamepadID, 0, 16),
		prev:  make([]ebiten.GamepadID, 0, 16),
		known: make([]Pad, 0, 16),
	}
}

// update follows the connected gamepads once a frame. A gamepad that the
// player uses while the menu gamepad is idle becomes the menu gamepad, and
// update reports that change. It allocates only when a gamepad connects or
// disconnects.
func (p *pads) update() (switched bool) {
	p.ids = p.src.appendIDs(p.ids[:0])
	if !slices.Equal(p.ids, p.prev) {
		p.prev = append(p.prev[:0], p.ids...)
		p.list()
	}
	var first, other ebiten.GamepadID
	hasFirst, hasOther, present, busy := false, false, false, false
	for _, id := range p.ids {
		if !p.src.standard(id) {
			continue
		}
		if !hasFirst {
			first, hasFirst = id, true
		}
		used := p.src.used(id)
		if p.menu.ok && id == p.menu.id {
			present, busy = true, used
		} else if used && !hasOther {
			other, hasOther = id, true
		}
	}
	switch {
	case hasOther && !busy:
		p.menu = pad{other, true}
		switched = true
	case !present:
		p.menu = pad{first, hasFirst}
	}
	p.pickDrive()
	return switched
}

// pickDrive puts the chosen gamepad, or else the menu gamepad, in drive.
func (p *pads) pickDrive() {
	p.drive = p.menu
	if i, ok := p.chosen(); ok {
		p.drive = pad{p.known[i].ID, true}
	}
}

// list builds known from the connected gamepads.
func (p *pads) list() {
	p.known = p.known[:0]
	for _, id := range p.ids {
		if !p.src.standard(id) {
			continue
		}
		guid, name := p.src.identity(id)
		n := 0
		for _, k := range p.known {
			if k.GUID == guid && k.Name == name {
				n++
			}
		}
		p.known = append(p.known, Pad{ID: id, GUID: guid, Name: name, Index: n})
	}
	p.gen++
}

// chosen returns the index in known of the chosen gamepad, and false when
// none is chosen or it is not connected.
func (p *pads) chosen() (int, bool) {
	if !p.want {
		return 0, false
	}
	return Match(p.known, p.wantGUID, p.wantName, p.wantIdx)
}

// Gamepad returns the ID of the gamepad of the driver controls, and false
// when no gamepad with the standard layout is connected.
func (r *Reader) Gamepad() (ebiten.GamepadID, bool) { return r.pads.drive.id, r.pads.drive.ok }

// Pads returns the connected gamepads with the standard layout, in order of
// ID, and a count that changes whenever the list changes. The slice is valid
// until the next Poll.
func (r *Reader) Pads() (pads []Pad, gen int) { return r.pads.known, r.pads.gen }

// Prefer chooses the gamepad of the driver controls by its GUID, name, and
// index, see Pad. While it is not connected, and with an empty name, the
// driver controls read the gamepad used last. The keyboard always works,
// and the menus read any gamepad.
func (r *Reader) Prefer(guid, name string, index int) {
	p := &r.pads
	p.want, p.wantGUID, p.wantName, p.wantIdx = name != "", guid, name, index
	p.pickDrive()
}

// Chosen returns the index in Pads of the chosen gamepad, and false when
// none is chosen or it is not connected.
func (r *Reader) Chosen() (int, bool) { return r.pads.chosen() }

// pressed reports whether button b of pad d is down.
func (p *pads) pressed(d pad, b ebiten.StandardGamepadButton) bool {
	return d.ok && p.src.pressed(d.id, b)
}

// justPressed reports whether button b of pad d went down in this frame.
func (p *pads) justPressed(d pad, b ebiten.StandardGamepadButton) bool {
	return d.ok && p.src.justPressed(d.id, b)
}

// value returns how far button b of pad d is down, from 0 to 1.
func (p *pads) value(d pad, b ebiten.StandardGamepadButton) float64 {
	if !d.ok {
		return 0
	}
	return p.src.value(d.id, b)
}
