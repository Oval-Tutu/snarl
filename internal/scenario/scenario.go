// Package scenario parses the scenario format in docs/scenario-format.md.
// A scenario is a timeline of RPM and throttle segments. The package does
// not depend on the engine, so that every tool can use it.
package scenario

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Kind is the type of a segment.
type Kind uint8

// Segment kinds.
const (
	// Hold keeps the RPM constant.
	Hold Kind = iota
	// Sweep moves the RPM linearly from RPM0 to RPM1.
	Sweep
)

// Segment is one line of a scenario.
type Segment struct {
	// Kind is Hold or Sweep.
	Kind Kind
	// RPM0 is the RPM at the start. RPM1 is the RPM at the end. For a
	// Hold, the two are equal.
	RPM0, RPM1 float64
	// Throttle is the throttle position, from 0 to 1.
	Throttle float64
	// Seconds is the duration of the segment.
	Seconds float64
	// Line is the line number in the source, from 1.
	Line int
}

// Scenario is a parsed scenario.
type Scenario struct {
	// Segments run in order.
	Segments []Segment
}

// ParseFile reads and parses the scenario file at path.
func ParseFile(path string) (*Scenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// Parse parses a scenario. An error names the line number.
func Parse(r io.Reader) (*Scenario, error) {
	var s Scenario
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		seg, err := parseSegment(fields)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		seg.Line = line
		s.Segments = append(s.Segments, seg)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("line %d: %w", line+1, err)
	}
	if len(s.Segments) == 0 {
		return nil, errors.New("scenario has no segments")
	}
	return &s, nil
}

func parseSegment(fields []string) (Segment, error) {
	var seg Segment
	var want int
	switch fields[0] {
	case "hold":
		seg.Kind, want = Hold, 4
	case "sweep":
		seg.Kind, want = Sweep, 5
	default:
		return seg, fmt.Errorf("unknown segment %q", fields[0])
	}
	if len(fields) != want {
		return seg, fmt.Errorf("%s needs %d values, got %d", fields[0], want-1, len(fields)-1)
	}
	v := make([]float64, want-1)
	for i, f := range fields[1:] {
		x, err := strconv.ParseFloat(f, 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
			return seg, fmt.Errorf("%q is not a number", f)
		}
		v[i] = x
	}
	if seg.Kind == Hold {
		seg.RPM0, seg.RPM1, seg.Throttle, seg.Seconds = v[0], v[0], v[1], v[2]
	} else {
		seg.RPM0, seg.RPM1, seg.Throttle, seg.Seconds = v[0], v[1], v[2], v[3]
	}
	switch {
	case seg.RPM0 <= 0 || seg.RPM1 <= 0:
		return seg, errors.New("RPM must be positive")
	case seg.Throttle < 0 || seg.Throttle > 1:
		return seg, errors.New("throttle must be from 0 to 1")
	case seg.Seconds <= 0:
		return seg, errors.New("seconds must be positive")
	}
	return seg, nil
}

// Duration returns the total duration in seconds.
func (s *Scenario) Duration() float64 {
	var d float64
	for _, seg := range s.Segments {
		d += seg.Seconds
	}
	return d
}

// At returns the RPM and throttle at time t in seconds. Before the start,
// it returns the first state. After the end, it returns the last state.
func (s *Scenario) At(t float64) (rpm, throttle float64) {
	start := 0.0
	for _, seg := range s.Segments {
		if t < start+seg.Seconds {
			f := max(t-start, 0) / seg.Seconds
			return seg.RPM0 + (seg.RPM1-seg.RPM0)*f, seg.Throttle
		}
		start += seg.Seconds
	}
	last := s.Segments[len(s.Segments)-1]
	return last.RPM1, last.Throttle
}
