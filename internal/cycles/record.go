// Package cycles builds and plays the cycle tables of an engine-sim engine:
// the synthesiser input of each exhaust channel over one 720-degree engine
// cycle, measured from engine-sim-render --dump-synth records on a grid of
// RPM and throttle holds, and a model of the part of the input that does
// not repeat from cycle to cycle. Playback reads the tables at the crank
// angle and feeds the engine-sim synthesiser port (internal/esynth).
// docs/research/cycle-tables.md describes the method.
package cycles

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/oval-tutu/snarl/internal/esynth"
)

// Record is a --dump-synth record with the crank angle and the segments
// of the render.
type Record struct {
	*esynth.Dump
	// Phase is the crank position after each step in engine cycles: it
	// grows by 1 per 720 degrees in the direction of rotation and does not
	// wrap. RPM is the crank speed after each step.
	Phase, RPM []float64
	// Timestep is the simulation step in seconds, Settle the pre-roll.
	Timestep, Settle float64
	Segments         []RecordSegment
	Cylinders        []RecordCylinder
}

// RecordSegment is one segment of the render, as the renderer ran it.
type RecordSegment struct {
	// Mode is hold, sweep, free or cut. A cut holds the RPM with the
	// ignition off.
	Mode       string  `json:"mode"`
	RPM0       float64 `json:"rpm0"`
	RPM1       float64 `json:"rpm1"`
	Throttle   float64 `json:"throttle"`
	Seconds    float64 `json:"seconds"`
	Start, End float64 `json:"-"`
}

// RecordCylinder is the exhaust path of one cylinder: its exhaust channel,
// the steps of its delay filter and its weight in the channel sum.
type RecordCylinder struct {
	ExhaustSystem int     `json:"exhaust_system"`
	DelaySteps    int     `json:"delay_steps"`
	Weight        float64 `json:"weight"`
}

type recordExtra struct {
	Crank     string           `json:"crank"`
	Timestep  float64          `json:"timestep_s"`
	Settle    float64          `json:"settle_s"`
	Segments  []RecordSegment  `json:"segments"`
	Cylinders []RecordCylinder `json:"cylinders"`
}

// LoadRecord reads the record in dir. The record must hold the crank angle
// (crank.f64), which the renderer writes since the cycle tables.
func LoadRecord(dir string) (*Record, error) {
	d, err := esynth.LoadDump(dir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "synth.json")) //nolint:gosec // The record directory is a user-supplied input.
	if err != nil {
		return nil, err
	}
	var x recordExtra
	if err := json.Unmarshal(b, &x); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	if x.Crank == "" || x.Timestep <= 0 || len(x.Segments) == 0 {
		return nil, fmt.Errorf("%s: the record has no crank angle, time step or segments; render it again", dir)
	}
	crank, err := readFloat64s(filepath.Join(dir, x.Crank))
	if err != nil {
		return nil, err
	}
	steps := len(d.Inputs[0])
	if len(crank) != 2*steps {
		return nil, fmt.Errorf("%s: %d crank values for %d steps", x.Crank, len(crank), steps)
	}
	r := &Record{Dump: d, Timestep: x.Timestep, Settle: x.Settle, Segments: x.Segments, Cylinders: x.Cylinders}
	r.Phase, r.RPM = unwrapCrank(crank)
	t := x.Settle
	for i := range r.Segments {
		r.Segments[i].Start = t
		t += r.Segments[i].Seconds
		r.Segments[i].End = t
	}
	return r, nil
}

// unwrapCrank turns the interleaved cycle angle (radians in 0 to 4 pi) and
// angular speed of each step into a phase in cycles that grows in the
// direction of rotation, and the RPM. engine-sim counts the cycle angle up
// when the crank turns clockwise (negative speed).
func unwrapCrank(crank []float64) (phase, rpm []float64) {
	n := len(crank) / 2
	phase = make([]float64, n)
	rpm = make([]float64, n)
	var sum float64
	direction := 0.0
	for i := range n {
		a, v := crank[2*i], crank[2*i+1]
		rpm[i] = math.Abs(v) * 60 / (2 * math.Pi)
		if direction == 0 && v != 0 {
			direction = -math.Copysign(1, v)
		}
		p := a / (4 * math.Pi)
		if direction < 0 {
			p = 1 - p
		}
		if i == 0 {
			sum = p
		} else {
			d := p - math.Mod(phase[i-1], 1)
			d -= math.Round(d)
			sum += d
		}
		phase[i] = sum
	}
	return phase, rpm
}

// Time returns the time after step i, from the start of the render.
func (r *Record) Time(i int) float64 {
	return float64(i+1) * r.Timestep
}

// ChannelDelays returns the delay of each exhaust channel in steps: the
// weighted mean of the delay filters of its cylinders.
func (r *Record) ChannelDelays() []float64 {
	out := make([]float64, r.InputChannelCount)
	w := make([]float64, r.InputChannelCount)
	for _, c := range r.Cylinders {
		if c.ExhaustSystem < 0 || c.ExhaustSystem >= len(out) {
			continue
		}
		out[c.ExhaustSystem] += c.Weight * float64(c.DelaySteps)
		w[c.ExhaustSystem] += c.Weight
	}
	for i := range out {
		if w[i] > 0 {
			out[i] /= w[i]
		}
	}
	return out
}

func readFloat64s(path string) ([]float64, error) {
	b, err := os.ReadFile(path) //nolint:gosec // The record directory is a user-supplied input.
	if err != nil {
		return nil, err
	}
	if len(b)%8 != 0 {
		return nil, errors.New(path + ": size is not a multiple of 8")
	}
	out := make([]float64, len(b)/8)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[i*8:]))
	}
	return out, nil
}
