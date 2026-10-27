// Command snarl-harness plays a snarl engine in real time. A game
// controller or the keyboard drives a simple drivetrain, and the screen
// shows the engine state and the audio timings. It can record the
// snapshots and the played audio, and it can write the snapshot log or the
// scenario (docs/scenario-format.md) of a scripted drive without a window.
//
// Usage:
//
//	snarl-harness [-profile NAME] [-catalogue DIR] [-record DIR]
//	snarl-harness [-profile NAME] [-catalogue DIR] [-script LOG.csv] [-scenario DRIVE.scn]
//
// The profile list is the five embedded profiles, then the catalogue
// profiles in DIR that have cycle tables, sorted by id. The harness skips a
// catalogue profile with no tables and notes it on screen. By default, DIR
// is profiles/catalogue in the working directory or the nearest parent up
// to the repository root.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/record"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/scope"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/stream"
)

const (
	screenW, screenH = 960, 620
	tps              = 60
	// flashTicks is the length of the pop flash on screen.
	flashTicks = tps / 4
)

// bufferBlocks are the player buffer sizes, in blocks, that the user can
// pick. defaultBuffer is the index of 6 blocks (32 ms), the PC figure of
// docs/research/platform.md section 7.
var bufferBlocks = []int{1, 2, 3, 4, 5, 6, 8, 10, 12, 16, 24, 32, 48, 64, 96}

const defaultBuffer = 5

// Gamepad buttons in the standard layout.
const (
	padThrottle = ebiten.StandardGamepadButtonFrontBottomRight
	padR1       = ebiten.StandardGamepadButtonFrontTopRight
	padL1       = ebiten.StandardGamepadButtonFrontTopLeft
	padA        = ebiten.StandardGamepadButtonRightBottom
	padX        = ebiten.StandardGamepadButtonRightLeft
	padLeft     = ebiten.StandardGamepadButtonLeftLeft
	padRight    = ebiten.StandardGamepadButtonLeftRight
	padUp       = ebiten.StandardGamepadButtonLeftTop
	padDown     = ebiten.StandardGamepadButtonLeftBottom
	padSelect   = ebiten.StandardGamepadButtonCenterLeft
	padStart    = ebiten.StandardGamepadButtonCenterRight
	legend      = "Throttle RT/Up/W  Shift RB,A/E up  LB,X/Q down  Profile D-pad L,R/Left,Right (all), 1-5 (embedded)  Gears Back/G\n" +
		"Ignition Start/Space  Buffer D-pad U,D/PgUp,PgDn  Record R  Own pops N  Pop character C  Meter and spectrum S"
	keyboardName = "keyboard"
)

// profileKeys select the embedded profiles, in the order of snarl.Profiles.
var profileKeys = []ebiten.Key{
	ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3,
	ebiten.KeyDigit4, ebiten.KeyDigit5,
}

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
	note     string // Why the catalogue profiles are missing, or "".
	eng      snarl.Engine
	engErr   error
	running  bool
	buffer   int // Index in bufferBlocks.
	snap     snarl.Snapshot
	shifts   shiftPops

	// noPops sets FlagNoPops, and character is the run-time pop
	// character. pops is the latest pop report, and flash the ticks left
	// of the pop flash.
	noPops    bool
	character snarl.PopCharacter
	pops      snarl.PopReport
	flash     int

	// scope measures the played audio, and hideScope hides it.
	// cylinders are the cylinder counts of the profiles, 0 when unknown.
	// scopeUpdate and scopeDraw are the mean times of the scope.
	scope                  *scope.Scope
	hideScope              bool
	cylinders              []int
	scopeUpdate, scopeDraw time.Duration

	quit   context.Context // Done on an interrupt signal.
	rec    *record.Recorder
	recDir string
	recMsg string // The result of the last recording.
}

func newHarness(quit context.Context, profile, catalogue, recDir string, recordNow bool) (*harness, error) {
	h := &harness{
		pads:    pads.NewReader(),
		drive:   drive.New(drive.DefaultGears),
		src:     stream.New(),
		scope:   scope.New(stream.SampleRate, stream.Block),
		running: true,
		buffer:  defaultBuffer,
		quit:    quit,
		recDir:  recDir,
	}
	var err error
	if h.profiles, h.profile, h.note, err = loadProfiles(profile, catalogue); err != nil {
		return nil, err
	}
	h.cylinders = cylinderCounts(h.profiles, catalogue)
	h.src.SetTap(h.scope)
	h.ctx = audio.NewContext(stream.SampleRate)
	h.swapEngine()
	if recordNow {
		h.toggleRecord()
	}
	return h, nil
}

// toggleRecord starts a recording, or stops the current one.
func (h *harness) toggleRecord() {
	if h.rec == nil {
		r, err := record.Start(h.recDir, time.Now(), stream.SampleRate, stream.Block)
		if err != nil {
			h.recMsg = fmt.Sprintf("Record error: %v", err)
			return
		}
		h.rec = r
		h.rec.Snapshot(h.profiles[h.profile].Name, snarl.Tier1, h.snap)
		h.src.SetTap(tee{h.scope, r})
		return
	}
	h.src.SetTap(h.scope)
	res, err := h.rec.Stop()
	h.rec = nil
	h.recMsg = fmt.Sprintf("Saved %s and %s: %v", res.CSVPath, filepath.Base(res.WAVPath), res)
	if err != nil {
		h.recMsg = fmt.Sprintf("Record error: %v (%v)", err, res)
	}
}

// swapEngine builds the engine of the selected profile, and
// hands it to the audio source, which crossfades to it.
func (h *harness) swapEngine() {
	e, err := snarl.New(h.profiles[h.profile], stream.SampleRate, snarl.Tier1)
	h.engErr = err
	if err != nil {
		return
	}
	e.Set(h.snap)
	h.eng = e
	h.pops = snarl.PopReport{}
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
	if h.quit.Err() != nil {
		return ebiten.Termination
	}
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
	if keyJust(ebiten.KeyR) {
		h.toggleRecord()
	}
	if keyJust(ebiten.KeyS) {
		h.hideScope = !h.hideScope
	}
	if !h.hideScope {
		h.updateScope()
	}

	dt := 1.0 / float64(ebiten.TPS())
	s := step(h.drive, &h.load, &h.shifts, h.profiles[h.profile], throttle, h.running, dt)
	if h.noPops {
		s.Flags |= snarl.FlagNoPops
	}
	s.PopCharacter = h.character
	h.snap = s
	h.flash = max(0, h.flash-1)
	if h.eng != nil {
		if r := h.eng.Pops(); r.Count != h.pops.Count {
			h.flash = flashTicks
			h.pops = r
		}
		h.eng.Set(s)
		if h.rec != nil {
			h.rec.Snapshot(h.profiles[h.profile].Name, snarl.Tier1, s)
		}
	}
	return nil
}

// shiftPops makes the pop requests of the gear changes, with the
// conditions of the exhaust effect of FiveHorizons: an upshift above
// 12 m/s with the throttle above 0.3, and a downshift above 20 m/s.
type shiftPops struct {
	up, down uint8   // Shift counters of the last step.
	rpm      float32 // RPM of the last step.
	request  uint8
	strength float32
}

// Speeds in metres per second and the throttle of the shift pop
// conditions.
const (
	upshiftSpeed    = 12
	upshiftThrottle = 0.3
	downshiftSpeed  = 20
)

// update counts a request for a qualifying shift of d since the last step.
// The strength of an upshift is the throttle times the RPM before the
// shift over the redline. The strength of a downshift is the RPM after
// the shift over the redline.
func (q *shiftPops) update(d *drive.Model, pr snarl.Profile, throttle float64, rpm float32) {
	if d.ShiftUp != q.up && d.Speed() > upshiftSpeed && throttle > upshiftThrottle {
		q.request++
		q.strength = min(1, float32(throttle)*q.rpm/pr.RedlineRPM)
	}
	if d.ShiftDown != q.down && d.Speed() > downshiftSpeed {
		q.request++
		q.strength = min(1, rpm/pr.RedlineRPM)
	}
	q.up, q.down, q.rpm = d.ShiftUp, d.ShiftDown, rpm
}

// step advances the drivetrain by dt seconds and returns the snapshot for
// the engine of profile pr, with the pop requests of the shifts.
func step(d *drive.Model, load *snarl.LoadFilter, q *shiftPops, pr snarl.Profile, throttle float64, running bool, dt float64) snarl.Snapshot {
	d.Step(throttle, running, dt)
	rpm := d.RPM(pr.IdleRPM, pr.LimiterRPM)
	overrun := d.Overrun()
	q.update(d, pr, throttle, rpm)
	s := snarl.Snapshot{
		RPM:         rpm,
		Throttle:    float32(throttle),
		Load:        load.Update(float32(throttle), float32(d.Accel()), float32(d.PeakAccel()), float32(dt), overrun),
		Speed:       float32(d.Speed()),
		Gear:        uint8(d.Gear()), //nolint:gosec // The gear is below drive.MaxGears.
		ShiftUp:     d.ShiftUp,
		ShiftDown:   d.ShiftDown,
		Limiter:     d.Limiter,
		PopRequest:  q.request,
		PopStrength: q.strength,
	}
	if running {
		s.Flags |= snarl.FlagRunning
	}
	if overrun {
		s.Flags |= snarl.FlagOverrun
	}
	if rpm >= min(pr.RedlineRPM, pr.LimiterRPM) {
		s.Flags |= snarl.FlagRedZone
	}
	if d.Gear() == d.Gears()-1 {
		s.Flags |= snarl.FlagFinalGear
	}
	return s
}

// updateSwitches reads the profile, gear count, ignition, and
// player buffer controls.
func (h *harness) updateSwitches() {
	p := h.pads
	n := len(h.profiles)
	profile := h.profile
	if p.JustPressed(padLeft) || keyJust(ebiten.KeyArrowLeft) {
		profile = stepProfile(profile, -1, n)
	}
	if p.JustPressed(padRight) || keyJust(ebiten.KeyArrowRight) {
		profile = stepProfile(profile, 1, n)
	}
	for i, k := range profileKeys {
		if i < n && keyJust(k) {
			profile = i
		}
	}
	if profile != h.profile {
		h.profile = profile
		h.swapEngine()
	}
	if p.JustPressed(padSelect) || keyJust(ebiten.KeyG) {
		h.drive.SetGears(drive.MinGears + (h.drive.Gears()-drive.MinGears+1)%(drive.MaxGears-drive.MinGears+1))
	}
	if p.JustPressed(padStart) || keyJust(ebiten.KeySpace) {
		h.running = !h.running
	}
	if keyJust(ebiten.KeyN) {
		h.noPops = !h.noPops
	}
	if keyJust(ebiten.KeyC) {
		h.character = (h.character + 1) % (snarl.CharacterRace + 1)
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
	say(8, "Profile %s (%d of %d, %s)   Ignition %s", pr.Name, h.profile+1, len(h.profiles),
		profileSource(h.profiles, h.profile), ignition)
	next()
	say(8, "%s", pr.Description)
	next()
	if h.engErr != nil {
		say(8, "Engine error: %v", h.engErr)
	} else if h.note != "" {
		say(8, "%s", h.note)
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
	h.drawPops(screen, y)
	next()
	own := "on"
	if h.noPops {
		own = "off"
	}
	say(8, "Own pops %s   Pop character %s   Pop requests %d", own, characterName(h.character), s.PopRequest)
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
	if h.rec != nil {
		say(8, "RECORDING %.1f s   dropped blocks %d   R stops", h.rec.Elapsed().Seconds(), h.rec.Dropped())
		if err := h.rec.Err(); err != nil {
			say(500, "Log error: %v", err)
		}
		next()
		say(8, "%s", h.rec.CSVPath)
		next()
		say(8, "%s", h.rec.WAVPath)
	} else {
		say(8, "Recording off   R records to %s", h.recDir)
		next()
		say(8, "%s", h.recMsg)
	}
	next()
	next()
	if !h.hideScope {
		y = h.drawScope(screen, y)
		next()
	}
	say(8, "%s", legend)
}

// drawPops draws the pop report at row y, with a flash after each pop.
func (h *harness) drawPops(screen *ebiten.Image, y int) {
	r := h.pops
	if h.flash > 0 {
		vector.FillRect(screen, 4, float32(y), 50, lineH-2, popColour(r.Source), false)
		ebitenutil.DebugPrintAt(screen, "POP", 18, y)
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Pops %d   latest %s from %s, strength %.2f",
		r.Count, r.Kind, r.Source, r.Strength), barX, y)
}

// popColour is the flash colour of a pop source.
func popColour(src snarl.PopSource) color.Color {
	switch src {
	case snarl.SourceRequest:
		return barFill
	case snarl.SourceLimiter:
		return markCut
	}
	return markRed
}

// characterName names a pop character.
func characterName(c snarl.PopCharacter) string {
	return [...]string{"profile", "stock", "sport", "race"}[c]
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// flags names the set flags of s, and the limiter cut, which the final
// gear does not make.
func flags(s snarl.Snapshot, cut bool) string {
	var b strings.Builder
	for _, f := range []struct {
		on   bool
		name string
	}{
		{s.Flags&snarl.FlagRunning != 0, "RUNNING"},
		{s.Flags&snarl.FlagOverrun != 0, "OVERRUN"},
		{s.Flags&snarl.FlagRedZone != 0, "RED-ZONE"},
		{s.Flags&snarl.FlagFinalGear != 0, "FINAL-GEAR"},
		{cut && s.Flags&snarl.FlagFinalGear == 0, "LIMITER-CUT"},
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
	profile := flag.String("profile", snarl.Profiles()[0], "start `profile`: an embedded profile or a catalogue id")
	catalogue := flag.String("catalogue", "", "catalogue profile `dir` (default profiles/catalogue in the working directory or a parent)")
	recDir := flag.String("record", "", "record from the start into `dir`; R starts and stops recordings in dir (default "+
		filepath.Join(os.TempDir(), "snarl-record")+")")
	script := flag.String("script", "", "write the snapshot log of the scripted drive, with shifts and lift-offs, to `file` and exit, with no window or audio")
	scn := flag.String("scenario", "", "write the RPM and the throttle of the scripted drive as a scenario to `file` and exit, with no window or audio")
	flag.Parse()
	if *catalogue == "" {
		*catalogue = findCatalogue()
	}
	var err error
	switch {
	case *script != "" || *scn != "":
		if *script != "" {
			err = writeScript(*script, *profile, *catalogue)
		}
		if *scn != "" {
			err = errors.Join(err, writeScenario(*scn, *profile, *catalogue))
		}
	default:
		err = play(*profile, *catalogue, *recDir)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// play runs the harness window. When recDir is not empty, it records from
// the start. An interrupt signal ends the harness, and the recording.
func play(profile, catalogue, recDir string) error {
	dir, recordNow := recDir, recDir != ""
	if !recordNow {
		dir = filepath.Join(os.TempDir(), "snarl-record")
	}
	quit, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	h, err := newHarness(quit, profile, catalogue, dir, recordNow)
	if err != nil {
		return err
	}
	ebiten.SetWindowTitle("snarl harness")
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetTPS(tps)
	err = ebiten.RunGame(h)
	if h.rec != nil {
		h.toggleRecord()
		log.Print(h.recMsg)
	}
	if errors.Is(err, ebiten.Termination) {
		return nil
	}
	return err
}
