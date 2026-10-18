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

// liftOffScript is the scripted drive: idle, then full throttle through
// the gears and lift off in second gear, then the same to third gear.
// Each lift-off lasts 3 seconds.
var liftOffScript = []scriptStep{
	{0, 0, 1},
	{0, 1, 2},
	{1, 1, 2},
	{1, 0, 3},
	{0, 1, 2},
	{1, 1, 2},
	{2, 1, 2},
	{2, 0, 3},
}

// writeScript writes the snapshot log of liftOffScript for the profile to
// path, as a recording would log it.
func writeScript(path, profile string) error {
	ps, i, err := loadProfiles(profile)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(runScript(f, ps[i], liftOffScript), f.Close())
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
			s := step(d, &load, pr, st.throttle, true, dt)
			if err := l.Write(at(), pr.Name, snarl.Tier1, s); err != nil {
				return err
			}
			tick++
		}
	}
	return l.Close(at())
}
