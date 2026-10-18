// Command snarl-harness plays a snarl engine in real time. A game
// controller or the keyboard drives a simple drivetrain, and the screen
// shows the engine state and the audio timings.
package main

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/drive"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/pads"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/stream"
)

const (
	screenW, screenH = 960, 400
	tps              = 60
)

// bufferBlocks are the player buffer sizes, in blocks, that the user can
// pick. defaultBuffer is the index of 6 blocks (32 ms), the PC figure of
// docs/research/platform.md section 7.
var bufferBlocks = []int{1, 2, 3, 4, 5, 6, 8, 10, 12, 16, 24, 32, 48, 64, 96}

const defaultBuffer = 5

// Gamepad buttons in the standard layout.
const (
	padThrottle  = ebiten.StandardGamepadButtonFrontBottomRight
	padR1        = ebiten.StandardGamepadButtonFrontTopRight
	padL1        = ebiten.StandardGamepadButtonFrontTopLeft
	padA         = ebiten.StandardGamepadButtonRightBottom
	padX         = ebiten.StandardGamepadButtonRightLeft
	padY         = ebiten.StandardGamepadButtonRightTop
	padLeft      = ebiten.StandardGamepadButtonLeftLeft
	padRight     = ebiten.StandardGamepadButtonLeftRight
	padUp        = ebiten.StandardGamepadButtonLeftTop
	padDown      = ebiten.StandardGamepadButtonLeftBottom
	padSelect    = ebiten.StandardGamepadButtonCenterLeft
	padStart     = ebiten.StandardGamepadButtonCenterRight
	legend       = "Throttle RT/Up/W  Shift RB,A/E up  LB,X/Q down  Profile D-pad L,R/1-4,Left,Right  Tier Y/T  Gears Back/G  Ignition Start/Space  Buffer D-pad U,D/PgUp,PgDn"
	keyboardName = "keyboard"
)

type harness struct {
	pads  *pads.Reader
	drive *drive.Model
	load  snarl.LoadFilter
	src   *stream.Source

	ctx      *audio.Context
	player   *audio.Player
	audioErr error

	profiles []snarl.Profile
	profile  int
	tier     snarl.Tier
	eng      snarl.Engine
	engErr   error
	running  bool
	buffer   int // Index in bufferBlocks.
	snap     snarl.Snapshot
}

func newHarness() (*harness, error) {
	h := &harness{
		pads:    pads.NewReader(),
		drive:   drive.New(drive.DefaultGears),
		src:     stream.New(),
		tier:    snarl.Tier1,
		running: true,
		buffer:  defaultBuffer,
	}
	for _, name := range snarl.Profiles() {
		p, err := snarl.LoadProfile(name)
		if err != nil {
			return nil, err
		}
		h.profiles = append(h.profiles, p)
	}
	h.ctx = audio.NewContext(stream.SampleRate)
	h.swapEngine()
	return h, nil
}

// swapEngine builds the engine of the selected profile and tier, and
// hands it to the audio source, which crossfades to it.
func (h *harness) swapEngine() {
	e, err := snarl.New(h.profiles[h.profile], stream.SampleRate, h.tier)
	h.engErr = err
	if err != nil {
		return
	}
	e.Set(h.snap)
	h.eng = e
	h.src.SetEngine(e)
}

// startAudio creates the player once the audio device is ready.
func (h *harness) startAudio() {
	if h.player != nil || h.audioErr != nil || !h.ctx.IsReady() {
		return
	}
	p, err := h.ctx.NewPlayerF32(h.src)
	if err != nil {
		h.audioErr = err
		return
	}
	h.player = p
	h.applyBuffer()
	p.Play()
}

func (h *harness) bufferTime() time.Duration {
	return time.Duration(bufferBlocks[h.buffer]) * stream.BlockTime
}

func (h *harness) applyBuffer() {
	if h.player != nil {
		h.player.SetBufferSize(h.bufferTime())
	}
	h.src.Reset()
}

func keyJust(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if inpututil.IsKeyJustPressed(k) {
			return true
		}
	}
	return false
}

func (h *harness) Update() error {
	h.pads.Poll()
	h.startAudio()
	p := h.pads

	throttle := p.Value(padThrottle)
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		throttle = 1
	}
	if p.JustPressed(padR1) || p.JustPressed(padA) || keyJust(ebiten.KeyE) {
		h.drive.Upshift()
	}
	if p.JustPressed(padL1) || p.JustPressed(padX) || keyJust(ebiten.KeyQ) {
		h.drive.Downshift()
	}
	h.updateSwitches()

	dt := 1.0 / float64(ebiten.TPS())
	h.drive.Step(throttle, h.running, dt)
	pr := h.profiles[h.profile]
	rpm := h.drive.RPM(pr.IdleRPM, pr.LimiterRPM)
	overrun := h.drive.Overrun()
	s := snarl.Snapshot{
		RPM:       rpm,
		Throttle:  float32(throttle),
		Load:      h.load.Update(float32(h.drive.Accel()), float32(h.drive.PeakAccel()), float32(dt), overrun),
		Speed:     float32(h.drive.Speed()),
		Gear:      uint8(h.drive.Gear()), //nolint:gosec // The gear is below drive.MaxGears.
		ShiftUp:   h.drive.ShiftUp,
		ShiftDown: h.drive.ShiftDown,
		Limiter:   h.drive.Limiter,
	}
	if h.running {
		s.Flags |= snarl.FlagRunning
	}
	if overrun {
		s.Flags |= snarl.FlagOverrun
	}
	if rpm >= min(pr.RedlineRPM, pr.LimiterRPM) {
		s.Flags |= snarl.FlagRedZone
	}
	h.snap = s
	if h.eng != nil {
		h.eng.Set(s)
	}
	return nil
}

// updateSwitches reads the profile, tier, gear count, ignition, and
// player buffer controls.
func (h *harness) updateSwitches() {
	p := h.pads
	n := len(h.profiles)
	profile := h.profile
	if p.JustPressed(padLeft) || keyJust(ebiten.KeyArrowLeft) {
		profile = (profile + n - 1) % n
	}
	if p.JustPressed(padRight) || keyJust(ebiten.KeyArrowRight) {
		profile = (profile + 1) % n
	}
	for i, k := range []ebiten.Key{ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4} {
		if i < n && keyJust(k) {
			profile = i
		}
	}
	tier := h.tier
	if p.JustPressed(padY) || keyJust(ebiten.KeyT) {
		tier = snarl.Tier1 - tier
	}
	if profile != h.profile || tier != h.tier {
		h.profile, h.tier = profile, tier
		h.swapEngine()
	}
	if p.JustPressed(padSelect) || keyJust(ebiten.KeyG) {
		h.drive.SetGears(drive.MinGears + (h.drive.Gears()-drive.MinGears+1)%(drive.MaxGears-drive.MinGears+1))
	}
	if p.JustPressed(padStart) || keyJust(ebiten.KeySpace) {
		h.running = !h.running
	}
	h.updateBuffer()
}

// updateBuffer reads the player buffer controls.
func (h *harness) updateBuffer() {
	p := h.pads
	buffer := h.buffer
	if p.JustPressed(padUp) || keyJust(ebiten.KeyPageUp) {
		buffer = min(buffer+1, len(bufferBlocks)-1)
	}
	if p.JustPressed(padDown) || keyJust(ebiten.KeyPageDown) {
		buffer = max(buffer-1, 0)
	}
	if buffer != h.buffer {
		h.buffer = buffer
		h.applyBuffer()
	}
}

var (
	barBack  = color.RGBA{0x30, 0x30, 0x38, 0xff}
	barFill  = color.RGBA{0x40, 0xa0, 0xe0, 0xff}
	barRed   = color.RGBA{0xe0, 0x40, 0x40, 0xff}
	markIdle = color.RGBA{0xa0, 0xa0, 0xa0, 0xff}
	markRed  = color.RGBA{0xff, 0xa0, 0x20, 0xff}
	markCut  = color.RGBA{0xff, 0x30, 0x30, 0xff}
)

const (
	barX, barW, barH = 100, 600, 12
	lineH            = 18
)

// bar draws a bar at row y, filled to share v from 0 to 1.
func bar(dst *ebiten.Image, y int, v float32, fill color.Color) {
	v = min(max(v, 0), 1)
	vector.FillRect(dst, barX, float32(y)+2, barW, barH, barBack, false)
	vector.FillRect(dst, barX, float32(y)+2, barW*v, barH, fill, false)
}

// mark draws a tick on the bar at row y at share v.
func mark(dst *ebiten.Image, y int, v float32, clr color.Color) {
	x := float32(barX) + barW*min(max(v, 0), 1)
	vector.FillRect(dst, x-1, float32(y), 2, barH+4, clr, false)
}

func (h *harness) Draw(screen *ebiten.Image) {
	pr := h.profiles[h.profile]
	s := h.snap
	scale := pr.LimiterRPM * 1.05
	y := 8
	say := func(x int, format string, a ...any) {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf(format, a...), x, y)
	}
	next := func() { y += lineH }

	ignition := "on"
	if !h.running {
		ignition = "off"
	}
	say(8, "Profile %s (%d of %d)   Tier %d   Ignition %s", pr.Name, h.profile+1, len(h.profiles), h.tier, ignition)
	next()
	say(8, "%s", pr.Description)
	next()
	if h.engErr != nil {
		say(8, "Engine error: %v", h.engErr)
	}
	next()

	fill := barFill
	if s.Flags&snarl.FlagRedZone != 0 {
		fill = barRed
	}
	say(8, "RPM")
	bar(screen, y, s.RPM/scale, fill)
	mark(screen, y, pr.IdleRPM/scale, markIdle)
	mark(screen, y, pr.RedlineRPM/scale, markRed)
	mark(screen, y, pr.LimiterRPM/scale, markCut)
	say(barX+barW+12, "%5.0f", s.RPM)
	next()
	say(barX, "idle %.0f (grey)  redline %.0f (orange)  limiter %.0f (red)", pr.IdleRPM, pr.RedlineRPM, pr.LimiterRPM)
	next()
	say(8, "Throttle")
	bar(screen, y, s.Throttle, barFill)
	say(barX+barW+12, "%3.0f%%", 100*s.Throttle)
	next()
	say(8, "Load")
	bar(screen, y, s.Load, barFill)
	say(barX+barW+12, "%3.0f%%", 100*s.Load)
	next()
	say(8, "Gear %d of %d   Speed %3.0f km/h", h.drive.Gear()+1, h.drive.Gears(), 3.6*s.Speed)
	next()
	say(8, "Flags %s   Shifts up %d down %d   Limiter cuts %d", flags(s, h.drive.Cut()), s.ShiftUp, s.ShiftDown, s.Limiter)
	next()
	next()

	st := h.src.Stats()
	block := stream.BlockTime
	pct := func(d time.Duration) float64 { return 100 * float64(d) / float64(block) }
	us := func(d time.Duration) float64 { return float64(d) / float64(time.Microsecond) }
	say(8, "Audio %d Hz   block %d frames (%.2f ms)   player buffer %d blocks (%.1f ms)",
		stream.SampleRate, stream.Block, ms(block), bufferBlocks[h.buffer], ms(h.bufferTime()))
	next()
	say(8, "Render per block: last %.0f us (%.1f%%)   mean %.0f us (%.1f%%)   max %.0f us (%.1f%%)",
		us(st.Last), pct(st.Last), us(st.Mean), pct(st.Mean), us(st.Max), pct(st.Max))
	next()
	say(8, "Max Read interval %.1f ms   blocks %d", ms(st.MaxGap), st.Blocks)
	next()
	switch {
	case h.audioErr != nil:
		say(8, "Audio error: %v", h.audioErr)
	case h.player == nil:
		say(8, "Audio: waiting for the device")
	}
	next()
	name, ok := h.pads.Name()
	if !ok {
		name = keyboardName
	}
	say(8, "Input %s", name)
	next()
	next()
	say(8, "%s", legend)
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// flags names the set flags of s, and the limiter cut.
func flags(s snarl.Snapshot, cut bool) string {
	var b strings.Builder
	for _, f := range []struct {
		on   bool
		name string
	}{
		{s.Flags&snarl.FlagRunning != 0, "RUNNING"},
		{s.Flags&snarl.FlagOverrun != 0, "OVERRUN"},
		{s.Flags&snarl.FlagRedZone != 0, "RED-ZONE"},
		{cut, "LIMITER-CUT"},
	} {
		if f.on {
			b.WriteString(f.name)
			b.WriteByte(' ')
		}
	}
	if b.Len() == 0 {
		return "none"
	}
	return strings.TrimSpace(b.String())
}

func (h *harness) Layout(int, int) (int, int) { return screenW, screenH }

func main() {
	h, err := newHarness()
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("snarl harness")
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetTPS(tps)
	if err := ebiten.RunGame(h); err != nil {
		log.Fatal(err)
	}
}
