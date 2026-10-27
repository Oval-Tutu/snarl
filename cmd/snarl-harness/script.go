package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/drive"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/record"
)

// scriptStep holds the gear and the throttle for some seconds. Gear 0 is
// first gear.
type scriptStep struct {
	gear              int
	throttle, seconds float64
}

// driveScript is the scripted drive: idle, full throttle through first
// gear to second, a lift-off and a downshift on the coast, full throttle
// through the gears to third, a lift-off and two downshifts on the coast,
// then part throttle with an upshift, full throttle and a last lift-off.
// Each upshift at full or part throttle and each downshift on the coast
// requests a pop.
var driveScript = []scriptStep{
	{0, 0, 1},
	{0, 1, 2.5},
	{1, 1, 2.5},
	{1, 0, 1},
	{0, 0, 1.5},
	{0, 1, 1.5},
	{1, 1, 2},
	{2, 1, 2.5},
	{2, 0, 1},
	{1, 0, 1},
	{0, 0, 1.5},
	{0, 0.5, 2},
	{1, 0.5, 2.5},
	{1, 1, 1.5},
	{1, 0, 3.5},
}

// writeScript writes the snapshot log of driveScript for the profile to
// path, as a recording would log it. The profile is an embedded profile or a
// profile of the catalogue directory.
func writeScript(path, profile, catalogue string) error {
	return writeDrive(path, profile, catalogue, runScript)
}

// writeScenario writes the RPM and the throttle of driveScript for the
// profile to path in the scenario format (docs/scenario-format.md), so that
// the engine-sim renderer and snarl-render can play the drive.
func writeScenario(path, profile, catalogue string) error {
	return writeDrive(path, profile, catalogue, runScenario)
}

// writeDrive writes driveScript for the profile to path with run.
func writeDrive(path, profile, catalogue string, run func(io.Writer, snarl.Profile, []scriptStep) error) error {
	ps, i, _, err := loadProfiles(profile, catalogue)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(run(f, ps[i], driveScript), f.Close())
}

// runScript logs the snapshot of each tick of steps at the tick time,
// with tier 1.
func runScript(w io.Writer, pr snarl.Profile, steps []scriptStep) error {
	l, err := record.NewLog(w)
	if err != nil {
		return err
	}
	tick := 0
	at := func() time.Duration { return time.Duration(tick) * time.Second / tps }
	if err := driveTicks(pr, steps, func(s snarl.Snapshot) error {
		err := l.Write(at(), pr.Name, snarl.Tier1, s)
		tick++
		return err
	}); err != nil {
		return err
	}
	return l.Close(at())
}

// runScenario writes the scenario of steps: for each tick, a sweep from the
// RPM of the tick before, idle at the start, to the RPM of the tick, with
// the throttle of the tick, over one tick. Ticks that keep the RPM of the
// tick before and the throttle join one hold.
func runScenario(w io.Writer, pr snarl.Profile, steps []scriptStep) error {
	b := bufio.NewWriter(w)
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	sec := func(ticks int) string { return strconv.FormatFloat(float64(ticks)/tps, 'g', -1, 64) }
	fmt.Fprintf(b, "# %s: written by snarl-harness -scenario from the scripted drive at %d ticks per second.\n", pr.Name, tps)
	b.WriteString("# Each sweep is one tick, from the RPM of the tick before to the RPM of the tick.\n")
	prev := pr.IdleRPM
	var holdThrottle float32
	holdTicks := 0
	flush := func() {
		if holdTicks > 0 {
			fmt.Fprintf(b, "hold %s %s %s\n", f(prev), f(holdThrottle), sec(holdTicks))
			holdTicks = 0
		}
	}
	if err := driveTicks(pr, steps, func(s snarl.Snapshot) error {
		if s.RPM == prev {
			if holdTicks > 0 && s.Throttle != holdThrottle {
				flush()
			}
			holdThrottle = s.Throttle
			holdTicks++
			return nil
		}
		flush()
		fmt.Fprintf(b, "sweep %s %s %s %s\n", f(prev), f(s.RPM), f(s.Throttle), sec(1))
		prev = s.RPM
		return nil
	}); err != nil {
		return err
	}
	flush()
	return b.Flush()
}

// driveTicks drives the harness drivetrain through steps at tps ticks per
// second with the harness LoadFilter, and calls tick with the snapshot of
// each tick.
func driveTicks(pr snarl.Profile, steps []scriptStep, tick func(snarl.Snapshot) error) error {
	d := drive.New(drive.DefaultGears)
	var load snarl.LoadFilter
	var shifts shiftPops
	const dt = 1.0 / tps
	for _, st := range steps {
		for moved := true; moved && d.Gear() != st.gear; {
			if d.Gear() < st.gear {
				moved = d.Upshift()
			} else {
				moved = d.Downshift()
			}
		}
		for range int(st.seconds * tps) {
			if err := tick(step(d, &load, &shifts, pr, st.throttle, true, dt)); err != nil {
				return err
			}
		}
	}
	return nil
}
