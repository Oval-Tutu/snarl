package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

const rate = 48000

// renderScenario renders the scenario text with the snapshots that
// snarl-render sets: load equals throttle, and the overrun flag is set
// below a throttle of 0.05.
func renderScenario(p snarl.Profile, text string) ([]float64, error) {
	scn, err := scenario.Parse(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	e, err := snarl.New(p, rate, snarl.Tier1)
	if err != nil {
		return nil, err
	}
	n := int(scn.Duration() * rate)
	buf := make([]float32, 256)
	out := make([]float64, 0, n)
	for i := 0; i < n; i += 256 {
		rpm, thr := scn.At(float64(i) / rate)
		s := snarl.Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: snarl.FlagRunning}
		if thr < 0.05 {
			s.Flags |= snarl.FlagOverrun
		}
		e.Set(s)
		b := buf[:min(256, n-i)]
		e.Render(b)
		for _, v := range b {
			out = append(out, float64(v))
		}
	}
	return out, nil
}

func loudness(p snarl.Profile, text string, from float64) (float64, error) {
	x, err := renderScenario(p, text)
	if err != nil {
		return 0, err
	}
	return analysis.IntegratedLoudness([][]float64{x[int(from*rate):]}, rate), nil
}

func parse(e *engine) (snarl.Profile, error) {
	return snarl.ParseProfile([]byte(profileTOML(e)))
}

// calibrate sets idle_gain_db so that the full-throttle hold at idle is
// idleGap LU below the hold at the top RPM, from 0 to 12 dB, and then
// output_db so that the sweep-wot scenario is at -16 LUFS, as
// TestOutputCalibration checks.
func calibrate(e *engine) error {
	e.OutputDB, e.IdleGainDB = 0, 0
	p, err := parse(e)
	if err != nil {
		return err
	}
	hold := func(rpm float64) (float64, error) {
		return loudness(p, fmt.Sprintf("hold %s 1 3\n", num(rpm)), 1)
	}
	idle, err := hold(e.IdleRPM)
	if err != nil {
		return err
	}
	top, err := hold(topRPM(e))
	if err != nil {
		return err
	}
	e.IdleGainDB = round(min(12, max(0, top-idle-idleGap)), 1)
	sweep := scenarioText(e, "sweep-wot")
	for range 8 {
		p, err := parse(e)
		if err != nil {
			return err
		}
		l, err := loudness(p, sweep, 0)
		if err != nil {
			return err
		}
		e.SweepLUFS = l
		if math.Abs(l+16) < 0.05 {
			return nil
		}
		e.OutputDB = round(min(40, max(-40, e.OutputDB-16-l)), 2)
	}
	if math.Abs(e.SweepLUFS+16) > 0.5 {
		e.Notes = append(e.Notes, fmt.Sprintf("Calibration stops at %.2f LUFS.", e.SweepLUFS))
	}
	return nil
}
