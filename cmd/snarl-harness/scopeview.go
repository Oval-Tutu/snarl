package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/scope"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/stream"
)

const (
	meterFloor = -60 // Level at the left end of the meter bar, in dBFS.
	specH      = 100 // Height of the spectrum in pixels.
)

// tee is a stream.Tap that sends each block to every tap in it.
type tee []stream.Tap

func (t tee) Capture(mono []float32) {
	for _, c := range t {
		c.Capture(mono)
	}
}

// cylinderCounts returns the cylinder count of each profile of the list
// that loadProfiles returns, from the profile files: the embedded profiles in
// the parent of the catalogue dir, and the catalogue profiles in dir. The
// count is 0 when the file is missing.
func cylinderCounts(ps []snarl.Profile, dir string) []int {
	n := make([]int, len(ps))
	if dir == "" {
		return n
	}
	for i, p := range ps {
		d := dir
		if i < len(snarl.Profiles()) {
			d = filepath.Dir(dir)
		}
		if data, err := os.ReadFile(filepath.Join(d, p.Name+".toml")); err == nil {
			n[i] = cylinders(data)
		}
	}
	return n
}

// cylinders counts the [[cylinder]] tables of a profile file.
func cylinders(data []byte) int {
	n := 0
	for line := range bytes.Lines(data) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("[[cylinder]]")) {
			n++
		}
	}
	return n
}

// firingHz returns the firing frequency of an engine of cyl cylinders at
// rpm: one pulse per cylinder every two revolutions.
func firingHz(rpm float32, cyl int) float64 {
	return float64(rpm) / 60 * float64(cyl) / 2
}

// meterShare maps a level in dBFS to a share of the meter bar.
func meterShare(db float32) float32 { return (db - meterFloor) / -meterFloor }

// updateScope reads the played audio into the scope and times it.
func (h *harness) updateScope() {
	start := time.Now()
	h.scope.Update()
	h.scopeUpdate = smooth(h.scopeUpdate, time.Since(start))
}

// smooth returns the running mean of the timings, over about 30 values.
func smooth(mean, d time.Duration) time.Duration { return mean + (d-mean)/30 }

// drawScope draws the level meter and the spectrum from row y, and returns
// the row after them.
func (h *harness) drawScope(screen *ebiten.Image, y int) int {
	start := time.Now()
	s := h.scope
	say := func(x, y int, format string, a ...any) {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf(format, a...), x, y)
	}
	say(8, y, "Level")
	bar(screen, y, meterShare(s.RMS), barFill)
	mark(screen, y, meterShare(s.Peak), markRed)
	mark(screen, y, meterShare(s.Hold), markCut)
	say(barX+barW+12, y, "RMS %5.1f  peak %5.1f  hold %5.1f", s.RMS, s.Peak, s.Hold)
	y += lineH
	say(barX, y, "dBFS from %d: RMS (bar), peak (orange), 1 s peak hold (red)", meterFloor)
	y += lineH

	say(8, y, "Spectrum")
	say(8, y+lineH, "0 dBFS")
	say(8, y+specH-lineH, "%d dBFS", scope.FloorDB)
	top := float32(y) + 2
	vector.FillRect(screen, barX, top, barW, specH, barBack, false)
	bw := float32(barW) / scope.Bands
	for b, l := range s.Levels {
		v := min(max((l-scope.FloorDB)/-scope.FloorDB, 0), 1)
		vector.FillRect(screen, barX+bw*float32(b)+1, top+specH*(1-v), bw-2, specH*v, barFill, false)
	}
	fire := 0.0
	if c := h.cylinders[h.profile]; c > 0 {
		fire = firingHz(h.snap.RPM, c)
	}
	if fire >= scope.LowHz && fire <= scope.HighHz {
		x := float32(barX) + barW*float32(scope.Position(fire))
		vector.FillRect(screen, x-1, top-2, 2, specH+4, markRed, false)
	}
	y += specH + 4
	for _, f := range []float64{30, 100, 300, 1000, 3000, 10000} {
		x := barX + int(barW*scope.Position(f))
		say(x, y, "%s", hzLabel(f))
	}
	y += lineH
	note := fmt.Sprintf("firing %.0f Hz (orange)", fire)
	if fire == 0 {
		note = "firing frequency unknown: no profile file"
	}
	h.scopeDraw = smooth(h.scopeDraw, time.Since(start))
	say(barX, y, "%d bands   %s   scope update %.0f us  draw %.0f us", scope.Bands, note,
		float64(h.scopeUpdate)/float64(time.Microsecond), float64(h.scopeDraw)/float64(time.Microsecond))
	return y + lineH
}

// hzLabel names an axis frequency, for example 300 or 3k.
func hzLabel(f float64) string {
	if f >= 1000 {
		return fmt.Sprintf("%.0fk", f/1000)
	}
	return fmt.Sprintf("%.0f", f)
}
