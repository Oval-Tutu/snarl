package main

import (
	_ "embed"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// The defaults for the snarl values that neither source has. README.md
// lists them.
const (
	defaultChamberM    = 0.3
	defaultChamberArea = 7.0
	// idleGap is the calibration target of idle_gain_db: the full-throttle
	// hold at the idle RPM is this many LU below the hold at the top RPM,
	// near the middle of the four tuned profiles.
	idleGap = 2.0
)

// sections are the fixed defaults of every imported profile, in profile
// order. The engine values go before them and the cylinders and paths
// after them.
const sections = `[pulse]
width_deg = 160
attack = 25
decay = 7
part_load_db = -0.8
cycle_variation = 0.04
jitter = 0.003
gain_variation = 0.12
timing_variation_deg = 2

[exhaust]
speed_of_sound = 565
crossover = "%s"
coupling = %s
outlet_direct = 0.4
steepening = 0.3
overlap = 1.0
low_shelf_db = 0
low_shelf_hz = 250

[noise]
am = 0.2
jet = 0.02
jet_hz = 1500

[pops]
density = 0.15
character = "sport"
gain = 1
body_hz = 200
body_ms = 35
jump_ms = 3
crack = 2.5
crack_ms = 16
weight_pop = 0.4
weight_boom = 0.2
weight_crackle = 0.3
weight_burble = 0.1

[overrun]
level_db = -1.2
idle_level_db = -1.2

[idle]
fade_rpm = %s
variation = 0.15
timing_deg = 1
imbalance = 0.25
hunt = 0.004
rock = 0.01
hunt_hz = 1.2

[voice]
gain = 1
output_db = %s
idle_gain_db = %s
idle_gain_rpm = 0
`

// pathDefaults are the path values that neither source has.
const pathDefaults = `reflection = -0.4
damping_hz = 2500
wall_loss_db = 2.5
wall_loss_hz = 2000
`

func num(x float64) string {
	return strconv.FormatFloat(round(x, 4), 'f', -1, 64)
}

// seed derives a fixed seed from the identifier.
func seed(id string) int {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int(h.Sum32()%65535) + 1
}

func profileTOML(e *engine) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Imported from %s by just import-profiles. Do not edit: change tools/import\n", e.Project)
	b.WriteString("# and run the import again. profiles/catalogue/README.md names the source file.\n")
	fmt.Fprintf(&b, "name = %q\n", e.ID)
	fmt.Fprintf(&b, "description = %q\n", e.Description)
	fmt.Fprintf(&b, "seed = %d\n", seed(e.ID))
	fmt.Fprintf(&b, "idle_rpm = %s\nredline_rpm = %s\n", num(e.IdleRPM), num(e.RedlineRPM))
	fmt.Fprintf(&b, "limiter_rpm = %s\nlimiter_kind = %q\nlimiter_ms = %s\n\n", num(e.LimiterRPM), e.LimiterKind, num(e.LimiterMS))
	fmt.Fprintf(&b, sections, e.Crossover, num(e.Coupling),
		num(e.IdleRPM+1400), num(e.OutputDB), num(e.IdleGainDB))
	for i, c := range e.Cylinders {
		fmt.Fprintf(&b, "\n[[cylinder]] # %d\ncrank_deg = %s\nbank = %d\nprimary_m = %s\n", i+1, num(c.CrankDeg), c.Bank, num(c.PrimaryM))
	}
	for _, p := range e.Paths {
		fmt.Fprintf(&b, "\n[[path]]\ncollector_m = %s\n%spipe_radius_m = %s\noutlet_radius_m = %s\n",
			num(p.CollectorM), pathDefaults, num(p.PipeRadiusM), num(p.OutletRadiusM))
		fmt.Fprintf(&b, "chamber_m = %s\nchamber_area_ratio = %s\ngain = %s\n", num(p.ChamberM), num(p.ChamberArea), num(p.Gain))
	}
	return b.String()
}

// topRPM is the highest RPM of the scenarios: the redline, or 100 rpm
// below the limiter when the limiter cuts at or below the redline.
func topRPM(e *engine) float64 {
	if e.LimiterRPM > 0 && e.LimiterRPM <= e.RedlineRPM {
		return e.LimiterRPM - 100
	}
	return e.RedlineRPM
}

// holdRPMs are the hold steps: idle, the multiples of 1000 rpm more than
// 500 rpm above idle and 300 rpm below the top, and the top.
func holdRPMs(e *engine) []float64 {
	top := topRPM(e)
	steps := []float64{e.IdleRPM}
	for r := 1000.0; r < top-300; r += 1000 {
		if r > e.IdleRPM+500 {
			steps = append(steps, r)
		}
	}
	return append(steps, top)
}

// scenarioText returns the named scenario of e, in the format of
// docs/scenario-format.md.
func scenarioText(e *engine, name string) string {
	top := num(topRPM(e))
	idle := num(e.IdleRPM)
	var b strings.Builder
	switch name {
	case "sweep-wot":
		fmt.Fprintf(&b, "# %s: full-throttle sweep from idle to the top rpm.\nsweep %s %s 1 8\n", e.ID, idle, top)
	case "overrun":
		fmt.Fprintf(&b, "# %s: full throttle at the top rpm, then closed throttle held, then a\n# closed-throttle sweep down to idle.\n", e.ID)
		fmt.Fprintf(&b, "hold %s 1 2\nhold %s 0 2\nsweep %s %s 0 6\n", top, top, top, idle)
	default:
		thr := map[string]string{"holds-t000": "0", "holds-t050": "0.5", "holds-t100": "1"}[name]
		fmt.Fprintf(&b, "# %s: 3 s holds at throttle %s, with 0.5 s sweeps between them.\n", e.ID, thr)
		steps := holdRPMs(e)
		for i, r := range steps {
			if i > 0 {
				fmt.Fprintf(&b, "sweep %s %s %s 0.5\n", num(steps[i-1]), num(r), thr)
			}
			fmt.Fprintf(&b, "hold %s %s 3\n", num(r), thr)
		}
	}
	return b.String()
}

var scenarioNames = []string{"holds-t000", "holds-t050", "holds-t100", "sweep-wot", "overrun"}

func writeScenarios(dir string, e *engine) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, s := range scenarioNames {
		if err := os.WriteFile(filepath.Join(dir, s+".scn"), []byte(scenarioText(e, s)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

//go:embed readme.md.tmpl
var readmeTemplate string

func writeReadme(path string, all []engine) error {
	t, err := template.New("readme").Funcs(template.FuncMap{
		"num":    num,
		"count":  func(e engine) int { return len(e.Cylinders) },
		"banks":  func(e engine) int { return len(e.Paths) },
		"notes":  func(e engine) string { return strings.Join(e.Notes, " ") },
		"source": func(p string) []engine { return filter(all, p) },
	}).Parse(readmeTemplate)
	if err != nil {
		return err
	}
	var b strings.Builder
	if err := t.Execute(&b, all); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func filter(all []engine, project string) []engine {
	var out []engine
	for _, e := range all {
		if e.Project == project {
			out = append(out, e)
		}
	}
	return out
}
