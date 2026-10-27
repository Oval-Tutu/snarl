package cycles

import (
	"errors"
	"fmt"
)

// The slices of a closing transient: the first starts closeStart seconds
// after the throttle closes, and each ends closeGrowth times later than it
// starts, at least closeMinSlice seconds and closeMinCycles engine cycles
// later, which holds at least two whole cycles.
const (
	closeStart     = 0.04
	closeGrowth    = 1.5
	closeMinSlice  = 0.04
	closeMinCycles = 3
	// CloseFrom is the least throttle before a close that counts as a
	// closing transient.
	CloseFrom = 0.5
)

// IsClose reports whether segment k of the record is a closing step: a
// hold at throttle 0 that follows a hold at the same RPM with a throttle of
// at least CloseFrom.
func (r *Record) IsClose(k int) bool {
	if k < 1 || k >= len(r.Segments) {
		return false
	}
	prev, seg := r.Segments[k-1], r.Segments[k]
	return prev.Mode == "hold" && seg.Mode == "hold" && prev.RPM0 == seg.RPM0 && prev.Throttle >= CloseFrom && seg.Throttle == 0
}

// CloseSlices measures the closing transient of segment k, a closing step
// (IsClose): the table point of each slice of the hold after the throttle
// closes, as Measure measures a hold, with Since the middle of the slice in
// seconds after the close. engine-sim leaves the steady states for a second
// or more after the throttle closes (docs/research/cycle-tables.md section
// 6.1), so the slices hold the input that no steady table holds.
func (r *Record) CloseSlices(k int, opt MeasureOptions) ([]Point, error) {
	if !r.IsClose(k) {
		return nil, fmt.Errorf("segment %d is not a closing step between holds at one RPM", k)
	}
	seg := r.Segments[k]
	cycle := 120 / seg.RPM0
	var out []Point
	for t := closeStart; ; {
		next := max(t*closeGrowth, t+closeMinSlice, t+closeMinCycles*cycle)
		if next > seg.Seconds {
			break
		}
		// A last slice shorter than the next step takes the rest of the hold.
		if after := max(next*closeGrowth, next+closeMinSlice, next+closeMinCycles*cycle); after > seg.Seconds {
			next = seg.Seconds
		}
		s := RecordSegment{Mode: "hold", RPM0: seg.RPM0, RPM1: seg.RPM0, Throttle: 0, Start: seg.Start + t, End: seg.Start + next}
		p, err := r.Measure(s, MeasureOptions{MaxHarmonics: opt.MaxHarmonics, NoAliases: opt.NoAliases, ModulationSlots: opt.ModulationSlots})
		if err != nil {
			return nil, fmt.Errorf("closing step at %g rpm, %.2f to %.2f s: %w", seg.RPM0, t, next, err)
		}
		p.Since = (t + next) / 2
		out = append(out, p)
		if next == seg.Seconds {
			break
		}
		t = next
	}
	if len(out) == 0 {
		return nil, errors.New("the closing step is too short for one slice")
	}
	return out, nil
}
