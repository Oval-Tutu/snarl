package main

import (
	"errors"
	"io"
	"os"
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
// path, as a recording would log it. The profile is a tuned profile or a
// profile of the catalogue directory.
func writeScript(path, profile, catalogue string) error {
	ps, i, _, err := loadProfiles(profile, catalogue)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(runScript(f, ps[i], driveScript), f.Close())
}

// runScript drives the harness drivetrain through steps at tps ticks per
// second with the harness LoadFilter, and logs the snapshot of each tick
// at the tick time, with tier 1.
func runScript(w io.Writer, pr snarl.Profile, steps []scriptStep) error {
	l, err := record.NewLog(w)
	if err != nil {
		return err
	}
	d := drive.New(drive.DefaultGears)
	var load snarl.LoadFilter
	var shifts shiftPops
	const dt = 1.0 / tps
	tick := 0
	at := func() time.Duration { return time.Duration(tick) * time.Second / tps }
	for _, st := range steps {
		for moved := true; moved && d.Gear() != st.gear; {
			if d.Gear() < st.gear {
				moved = d.Upshift()
			} else {
				moved = d.Downshift()
			}
		}
		for range int(st.seconds * tps) {
			s := step(d, &load, &shifts, pr, st.throttle, true, dt)
			if err := l.Write(at(), pr.Name, snarl.Tier1, s); err != nil {
				return err
			}
			tick++
		}
	}
	return l.Close(at())
}
