package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// exhaustSpec and pathSpec hold the number keys of the [exhaust] and
// [[path]] tables of a profile, as model.ExhaustSpec and model.PathSpec
// hold them at 195ba4e. They are copies, so that this tool describes the
// exhaust path of that commit while the model changes.
type exhaustSpec struct {
	SpeedOfSound, Coupling, OutletDirect, Steepening, Overlap float64
	LowShelfDB, LowShelfHz                                    float64
}

type pathSpec struct {
	CollectorM, Reflection, DampingHz, PipeRadiusM, OutletM float64
	WallLossDB, WallLossHz, ChamberM, ChamberArea, Gain     float64
	ListenerM                                               float64
}

// defaultExhaust returns the exhaust defaults of model.DefaultSpec at
// 195ba4e.
func defaultExhaust() exhaustSpec {
	return exhaustSpec{SpeedOfSound: 565, OutletDirect: 0.4, Steepening: 0.3, LowShelfHz: 250}
}

// readExhaust reads the [exhaust] table and the [[path]] tables of a
// profile over the model defaults. It reads only the number keys of those
// tables, the subset of the profile format that the path response needs,
// and skips every other table. An unknown key in those tables is an
// error, so that a new path key is not lost without notice.
func readExhaust(r io.Reader) (exhaustSpec, []pathSpec, error) {
	e := defaultExhaust()
	var paths []pathSpec
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
				paths = append(paths, pathSpec{})
				keys = pathKeys(&paths[len(paths)-1])
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

func pathKeys(p *pathSpec) map[string]*float64 {
	return map[string]*float64{
		"collector_m": &p.CollectorM, "reflection": &p.Reflection, "damping_hz": &p.DampingHz,
		"pipe_radius_m": &p.PipeRadiusM, "outlet_radius_m": &p.OutletM,
		"wall_loss_db": &p.WallLossDB, "wall_loss_hz": &p.WallLossHz,
		"chamber_m": &p.ChamberM, "chamber_area_ratio": &p.ChamberArea,
		"gain": &p.Gain, "listener_m": &p.ListenerM,
	}
}

// stage names one linear stage of a snarl exhaust path.
type stage int

const (
	stageCollector stage = iota
	stageWall
	stageLowShelf
	stageChamber
	stageMode
	stageOutlet
	stageCount
)

var stageNames = [stageCount]string{"collector", "wall loss", "low shelf", "chamber", "mode cutoff", "outlet"}

// stageCost is the multiply-adds per sample of each stage, counted from
// internal/model/path.go: the collector one-pole (2) and its feedback (1),
// a first-order shelf (3), the chamber feedback and transmission (2),
// two biquads (10), and the outlet biquad with its direct mix (5 + 2).
var stageCost = [stageCount]int{3, 3, 3, 2, 10, 7}

// pathImpulse returns n samples of the impulse response at rate of the
// linear stages of the path p that on selects, built from the dsp parts
// as internal/model/path.go builds them for the engine wave: the
// collector reflection with its damping, the wall-loss and low shelves,
// the expansion chamber, the plane-mode cutoff and the outlet. It leaves
// out the wave steepening (not linear), the path gain and the air delay
// (neither changes the shape).
func pathImpulse(p *pathSpec, e *exhaustSpec, rate float64, n int, on [stageCount]bool) []float64 {
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
		if on[stageCollector] {
			y += colG * colLP.Process(col.Read(colD-1))
			col.Write(y)
		}
		if on[stageWall] {
			y = wall.Process(y)
		}
		if on[stageLowShelf] {
			y = low.Process(y)
		}
		if on[stageChamber] && ch != nil {
			v := y + chR2*ch.Read(2*chD-1)
			ch.Write(v)
			y = chT * ch.Read(chD)
		}
		if on[stageMode] {
			y = mode2.Process(mode1.Process(y))
		}
		if on[stageOutlet] {
			y = direct*y + outlet.Process(y)
		}
		h[i] = float64(y)
	}
	return h
}

// allStages selects every stage.
func allStages() [stageCount]bool {
	var on [stageCount]bool
	for i := range on {
		on[i] = true
	}
	return on
}

// onlyStage selects one stage.
func onlyStage(s stage) [stageCount]bool {
	var on [stageCount]bool
	on[s] = true
	return on
}
