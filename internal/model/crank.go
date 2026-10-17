package model

import "sort"

// firing is one entry of the firing schedule.
type firing struct {
	// offset is the firing point as a fraction of the 720-degree cycle.
	offset float32
	cyl    int
}

// crank is the crank phasor and the firing schedule. The phase is a
// fraction of the 720-degree cycle, from 0 to 1.
type crank struct {
	phase  float32
	events []firing
	next   int
	// past is how far the phase ran beyond the last firing, in cycles.
	past float32
}

func newCrank(offsets []float32) crank {
	ev := make([]firing, len(offsets))
	for i, o := range offsets {
		ev[i] = firing{offset: o, cyl: i}
	}
	sort.SliceStable(ev, func(a, b int) bool { return ev[a].offset < ev[b].offset })
	return crank{events: ev}
}

// step advances the phase by inc cycles. It returns the cylinder that
// fires in this sample, or -1. The increment must stay below the smallest
// gap between two firings, so at most one firing falls in one sample.
func (c *crank) step(inc float32) int {
	c.phase += inc
	if c.next == len(c.events) {
		if c.phase < 1 {
			return -1
		}
		c.phase--
		c.next = 0
	}
	e := c.events[c.next]
	if c.phase >= e.offset {
		c.next++
		c.past = c.phase - e.offset
		return e.cyl
	}
	return -1
}
