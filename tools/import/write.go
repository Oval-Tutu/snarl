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

	"github.com/oval-tutu/snarl/internal/analysis"
)

// The defaults for the snarl values that neither source has. README.md
// lists them. The chamber of an engine-sim path is the chamber of the
// EngineLab path stages of the earlier model, which the pop bank follows.
const (
	defaultChamberM    = 0.3
	defaultChamberArea = 7.0
)

// sections are the fixed defaults of every imported profile, in profile
// order. The engine values go before them and the cylinders and paths
// after them.
const sections = `[pulse]
table = "%s"
cycle_variation = 0.04
jitter = 0.003
gain_variation = 0.12
timing_variation_deg = 2

[exhaust]
speed_of_sound = 565
crossover = "%s"
coupling = %s
formant_gain_db = %s
pop_formant_gain_db = %s
pop_delay_ms = %s

[noise]
hf_gain = %s
am = %s
am_cutoff_hz = 2000
input_jitter = %s

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

[idle]
fade_rpm = %s
hunt = 0.004
hunt_hz = 1.2

[voice]
gain = 1
leveller_db = %s
output_db = %s
`

// pathDefaults are the path values that neither source has.
const pathDefaults = `reflection = -0.4
damping_hz = 2500
`

// pulseFor returns the pulse table of the reference engine closest in
// cylinder count to an engine of n cylinders: i4-vtec up to 5 cylinders,
// v8-ls from 6 to 9, and v12-412 from 10, whose table spans 3810 to 18000
// rpm. A catalogue engine has no reference with a CSV row per step, which
// the pulse measurement needs, so every catalogue profile takes the table
// of a reference engine.
func pulseFor(n int) string {
	switch {
	case n <= 5:
		return "i4-vtec"
	case n <= 9:
		return "v8-ls"
	}
	return "v12-412"
}

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
	fmt.Fprintf(&b, sections, e.PulseTable, e.Crossover, num(e.Coupling),
		num(round(e.Bank.GainDB, 2)), num(round(e.PopBank.GainDB, 2)), num(round(e.PopDelayMS, 3)),
		num(e.HFGain), num(e.AM), num(e.InputJitter),
		num(e.IdleRPM+1400), num(e.LevellerDB), num(e.OutputDB))
	for i, c := range e.Cylinders {
		fmt.Fprintf(&b, "\n[[cylinder]] # %d\ncrank_deg = %s\nbank = %d\nprimary_m = %s\n", i+1, num(c.CrankDeg), c.Bank, num(c.PrimaryM))
	}
	for _, p := range e.Paths {
		fmt.Fprintf(&b, "\n[[path]]\ncollector_m = %s\n%sgain = %s\n", num(p.CollectorM), pathDefaults, num(p.Gain))
	}
	fmt.Fprintf(&b, "\n# Formant bank fitted to the engine-sim impulse response %s.\n", e.ImpulseResponse)
	// A strings.Builder never returns a write error.
	_ = analysis.WriteBankTOML(&b, "formant", &e.Bank)
	b.WriteString("\n# Pop bank fitted to the EngineLab pop stages of the earlier model.\n")
	_ = analysis.WriteBankTOML(&b, "pop_formant", &e.PopBank)
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
