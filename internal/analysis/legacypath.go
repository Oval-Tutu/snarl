package analysis

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// LegacyExhaust and LegacyPath hold the number keys of the [exhaust] and
// [[path]] tables of a profile with the EngineLab exhaust path stages, as
// internal/model/path.go read them at a73b874: the collector, the wall
// loss and low shelves, the expansion chamber, the mode cutoff and the
// outlet. They are copies, so that the formant fit and the pop tests can
// describe that path after the model dropped it.
type LegacyExhaust struct {
	SpeedOfSound, Coupling, OutletDirect, Steepening, Overlap float64
	LowShelfDB, LowShelfHz                                    float64
}

// LegacyPath is one [[path]] table of a LegacyExhaust profile.
type LegacyPath struct {
	CollectorM, Reflection, DampingHz, PipeRadiusM, OutletM float64
	WallLossDB, WallLossHz, ChamberM, ChamberArea, Gain     float64
	ListenerM                                               float64
}

// defaultLegacyExhaust returns the exhaust defaults of model.DefaultSpec
// at a73b874.
func defaultLegacyExhaust() LegacyExhaust {
	return LegacyExhaust{SpeedOfSound: 565, OutletDirect: 0.4, Steepening: 0.3, LowShelfHz: 250}
}

// ReadLegacyExhaust reads the [exhaust] table and the [[path]] tables of a
// profile over the model defaults. It reads only the number keys of those
// tables, the subset of the profile format that the path response needs,
// and skips every other table and the crossover key. An unknown key in
// those tables is an error, so that a new path key is not lost without
// notice.
func ReadLegacyExhaust(r io.Reader) (LegacyExhaust, []LegacyPath, error) {
	e := defaultLegacyExhaust()
	var paths []LegacyPath
	exhaustKeys := map[string]*float64{
		"speed_of_sound": &e.SpeedOfSound, "coupling": &e.Coupling,
		"outlet_direct": &e.OutletDirect, "steepening": &e.Steepening,
		"overlap": &e.Overlap, "low_shelf_db": &e.LowShelfDB, "low_shelf_hz": &e.LowShelfHz,
	}
	var keys map[string]*float64
	section := ""
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		text = strings.TrimSpace(text)
		switch {
		case text == "":
			continue
		case strings.HasPrefix(text, "["):
			section = text
			keys = nil
			switch section {
			case "[exhaust]":
				keys = exhaustKeys
			case "[[path]]":
				paths = append(paths, LegacyPath{})
				keys = legacyPathKeys(&paths[len(paths)-1])
			}
			continue
		}
		if keys == nil {
			continue
		}
		k, v, ok := strings.Cut(text, "=")
		if !ok {
			return e, nil, fmt.Errorf("line %d: no = in %q", line, text)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if section == "[exhaust]" && k == "crossover" {
			continue
		}
		dst, known := keys[k]
		if !known {
			return e, nil, fmt.Errorf("line %d: unknown key %q in %s", line, k, section)
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return e, nil, fmt.Errorf("line %d: %s: %w", line, k, err)
		}
		*dst = f
	}
	if err := sc.Err(); err != nil {
		return e, nil, err
	}
	for i := range paths {
		if paths[i].Gain == 0 {
			paths[i].Gain = 1
		}
	}
	return e, paths, nil
}

func legacyPathKeys(p *LegacyPath) map[string]*float64 {
	return map[string]*float64{
		"collector_m": &p.CollectorM, "reflection": &p.Reflection, "damping_hz": &p.DampingHz,
		"pipe_radius_m": &p.PipeRadiusM, "outlet_radius_m": &p.OutletM,
		"wall_loss_db": &p.WallLossDB, "wall_loss_hz": &p.WallLossHz,
		"chamber_m": &p.ChamberM, "chamber_area_ratio": &p.ChamberArea,
		"gain": &p.Gain, "listener_m": &p.ListenerM,
	}
}

// LegacyStage names one linear stage of a legacy exhaust path.
type LegacyStage int

// The linear stages of a legacy exhaust path, in path order.
const (
	StageCollector LegacyStage = iota
	StageWall
	StageLowShelf
	StageChamber
	StageMode
	StageOutlet
	LegacyStageCount
)

// LegacyStageNames names the stages.
var LegacyStageNames = [LegacyStageCount]string{"collector", "wall loss", "low shelf", "chamber", "mode cutoff", "outlet"}

// LegacyStageCost is the multiply-adds per sample of each stage: the
// collector one-pole (2) and its feedback (1), a first-order shelf (3), the
// chamber feedback and transmission (2), two biquads (10), and the outlet
// biquad with its direct mix (5 + 2).
var LegacyStageCost = [LegacyStageCount]int{3, 3, 3, 2, 10, 7}

// LegacyAllStages selects every stage.
func LegacyAllStages() [LegacyStageCount]bool {
	var on [LegacyStageCount]bool
	for i := range on {
		on[i] = true
	}
	return on
}

// LegacyOnlyStage selects one stage.
func LegacyOnlyStage(s LegacyStage) [LegacyStageCount]bool {
	var on [LegacyStageCount]bool
	on[s] = true
	return on
}

// LegacyPopStages selects the stages of a legacy path that the pop bank of
// the model takes over: every linear stage except the collector, which the
// model keeps per path.
func LegacyPopStages() [LegacyStageCount]bool {
	on := LegacyAllStages()
	on[StageCollector] = false
	return on
}

// LegacyPopImpulse returns n samples of the impulse response at rate of
// the pop stages of the first path, which every path must share. Paths may
// differ in their collector, gain and listener distance.
func LegacyPopImpulse(paths []LegacyPath, e *LegacyExhaust, rate float64, n int) ([]float64, error) {
	if len(paths) == 0 {
		return nil, errors.New("no [[path]] table")
	}
	for i := 1; i < len(paths); i++ {
		a, b := paths[0], paths[i]
		a.CollectorM, a.Reflection, a.DampingHz, a.Gain, a.ListenerM = 0, 0, 0, 0, 0
		b.CollectorM, b.Reflection, b.DampingHz, b.Gain, b.ListenerM = 0, 0, 0, 0, 0
		if a != b {
			return nil, fmt.Errorf("path %d has other pop stage keys than path 1, so one pop bank cannot serve both", i+1)
		}
	}
	return LegacyPathImpulse(&paths[0], e, rate, n, LegacyPopStages()), nil
}

// LegacyPopDelayMS returns the delay in milliseconds of the expansion
// chamber of the path, which a pop bank that follows the magnitude of the
// pop stages leaves out, or 0 for a path with no chamber.
func LegacyPopDelayMS(p *LegacyPath, e *LegacyExhaust) float64 {
	if p.ChamberM <= 0 || p.ChamberArea <= 1 {
		return 0
	}
	return 1000 * p.ChamberM / e.SpeedOfSound
}

// LegacyPathImpulse returns n samples of the impulse response at rate of
// the linear stages of the path p that on selects, built from the dsp
// parts as internal/model/path.go built them for the engine wave and the
// pops at a73b874: the collector reflection with its damping, the
// wall-loss and low shelves, the expansion chamber, the plane-mode cutoff
// and the outlet. It leaves out the wave steepening (not linear), the
// path gain and the air delay (neither changes the shape).
func LegacyPathImpulse(p *LegacyPath, e *LegacyExhaust, rate float64, n int, on [LegacyStageCount]bool) []float64 {
	c := e.SpeedOfSound
	colD := max(1, int(math.Round(2*p.CollectorM/c*rate)))
	col := dsp.NewDelay(colD)
	colLP := dsp.OnePole{A: dsp.OnePoleCoef(p.DampingHz, rate)}
	if p.DampingHz <= 0 {
		colLP.A = 1
	}
	colG := float32(p.Reflection)
	wallHz := p.WallLossHz
	if wallHz <= 0 {
		wallHz = 1500
	}
	wall := dsp.NewHighShelf(wallHz, math.Pow(10, -p.WallLossDB/20), rate)
	low := dsp.NewLowShelf(e.LowShelfHz, math.Pow(10, e.LowShelfDB/20), rate)
	var ch *dsp.Delay
	var chD int
	var chR2, chT float32
	if p.ChamberM > 0 && p.ChamberArea > 1 {
		chD = max(1, int(math.Round(p.ChamberM/c*rate)))
		ch = dsp.NewDelay(2 * chD)
		r := (p.ChamberArea - 1) / (p.ChamberArea + 1)
		chR2 = float32(r * r * 0.97)
		chT = float32(1 - r*r)
	}
	var mode1, mode2, outlet dsp.Biquad
	fc := 1.8412 * c / (2 * math.Pi * p.PipeRadiusM)
	mode1.SetLowPass(fc, 0.5412, rate)
	mode2.SetLowPass(fc, 1.3066, rate)
	outlet.SetOutlet(p.OutletM, c, rate)
	direct := float32(e.OutletDirect)

	h := make([]float64, n)
	for i := range h {
		var y float32
		if i == 0 {
			y = 1
		}
		if on[StageCollector] {
			y += colG * colLP.Process(col.Read(colD-1))
			col.Write(y)
		}
		if on[StageWall] {
			y = wall.Process(y)
		}
		if on[StageLowShelf] {
			y = low.Process(y)
		}
		if on[StageChamber] && ch != nil {
			v := y + chR2*ch.Read(2*chD-1)
			ch.Write(v)
			y = chT * ch.Read(chD)
		}
		if on[StageMode] {
			y = mode2.Process(mode1.Process(y))
		}
		if on[StageOutlet] {
			y = direct*y + outlet.Process(y)
		}
		h[i] = float64(y)
	}
	return h
}
