package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// The full-range scenarios hold each throttle of the cycle table grid at
// every multiple of fullRangeStep from idle to the limiter, in one file per
// throttle, so that one engine-sim render covers a throttle row.
const (
	fullRangeStep = 250
	// fullRangeSettle is the settle time in seconds at the start of each
	// hold, which the windows of the full-range scenarios skip.
	fullRangeSettle = 0.5
	// fullRangeCycles and fullRangeMinHold set the hold time after the
	// settle time: at least this many whole cycles and seconds, as in the
	// grid of tools/cycles scenarios.
	fullRangeCycles  = 24
	fullRangeMinHold = 1.0
	// fullRangeSweep is the length in seconds of the sweep between holds.
	fullRangeSweep = 0.25
	// fullRangeLimiter is the share of the limiter RPM that the top hold
	// stays below: the snarl limiter starts its bounce at 0.998 of it.
	fullRangeLimiter = 0.998
)

// fullRangeThrottles are the throttle rows of the cycle table grid.
var fullRangeThrottles = []float64{0, 0.25, 0.5, 0.75, 1}

// fullRangeName returns the scenario name of the full-range file at the
// throttle th.
func fullRangeName(th float64) string {
	return fmt.Sprintf("fullrange-t%03d", int(math.Round(th*100)))
}

// fullRangeRPMs returns idle, the multiples of fullRangeStep above it, and
// none at or above fullRangeLimiter of the limiter RPM.
func fullRangeRPMs(idle, limiter float64) []float64 {
	out := []float64{idle}
	for r := math.Floor(idle/fullRangeStep)*fullRangeStep + fullRangeStep; r < fullRangeLimiter*limiter; r += fullRangeStep {
		if r-idle >= fullRangeStep/4 {
			out = append(out, r)
		}
	}
	return out
}

// fullRangeHold returns the length in seconds of a full-range hold at rpm.
func fullRangeHold(rpm float64) float64 {
	s := fullRangeSettle + max(fullRangeMinHold, fullRangeCycles*120/rpm)
	return math.Ceil(s*10) / 10
}

// fullRangeGrid returns the profile of the engine and the RPM of its
// full-range holds: idle is the first hold of
// <scenario-dir>/<engine>/holds-t100.scn and the top is below the limiter
// RPM of the profile.
func (c *config) fullRangeGrid(engine string) (snarl.Profile, []float64, error) {
	p, err := snarl.LoadProfile(engine)
	if err != nil {
		return p, nil, err
	}
	if p.LimiterRPM <= 0 {
		return p, nil, fmt.Errorf("%s: no limiter RPM", engine)
	}
	holds := filepath.Join(c.scn, engine, "holds-t100.scn")
	sc, err := scenario.ParseFile(holds)
	if err != nil {
		return p, nil, err
	}
	if len(sc.Segments) == 0 || sc.Segments[0].Kind != scenario.Hold {
		return p, nil, fmt.Errorf("%s: the first segment is not a hold", holds)
	}
	return p, fullRangeRPMs(sc.Segments[0].RPM0, float64(p.LimiterRPM)), nil
}

// writeFullRange writes the full-range scenarios of the engine into
// <scenario-dir>/<engine>/.
func (c *config) writeFullRange(engine string) error {
	p, rpms, err := c.fullRangeGrid(engine)
	if err != nil {
		return err
	}
	write := func(name, what string, th float64, rpms []float64) error {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s: written by tools/scorecard --write-fullrange. Holds at throttle %g %s\n", engine, th, what)
		fmt.Fprintf(&b, "# from idle (the first hold of holds-t100.scn) to below the limiter at %g rpm.\n", p.LimiterRPM)
		fmt.Fprintf(&b, "# Each hold is %g s of settling and at least %d whole cycles and %g s; %g s sweeps join the holds.\n",
			fullRangeSettle, fullRangeCycles, fullRangeMinHold, fullRangeSweep)
		for i, r := range rpms {
			if i > 0 {
				fmt.Fprintf(&b, "sweep %g %g %g %g\n", rpms[i-1], r, th, fullRangeSweep)
			}
			fmt.Fprintf(&b, "hold %g %g %g\n", r, th, fullRangeHold(r))
		}
		return os.WriteFile(filepath.Join(c.scn, engine, name+".scn"), []byte(b.String()), 0o644)
	}
	for _, th := range fullRangeThrottles {
		if err := write(fullRangeName(th), fmt.Sprintf("every %d rpm", fullRangeStep), th, rpms); err != nil {
			return err
		}
	}
	// The mid file holds full throttle halfway between the RPM of the
	// other files, where the engine plays two neighbouring grid points.
	var mid []float64
	for i := 1; i < len(rpms); i++ {
		mid = append(mid, (rpms[i-1]+rpms[i])/2)
	}
	return write(fullRangeMid, "halfway between those of the other files", 1, mid)
}

// fullRangeMid is the scenario name of the full-throttle holds halfway
// between the full-range RPM.
const fullRangeMid = "fullrange-mid-t100"
