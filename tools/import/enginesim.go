package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// esDump is the JSON that engine-sim-render --dump-json writes.
type esDump struct {
	RedlineRPM  float64 `json:"redline_rpm"`
	RevLimitRPM float64 `json:"rev_limit_rpm"`
	LimiterS    float64 `json:"limiter_duration_s"`
	Idle        struct {
		MeasuredRPM float64 `json:"measured_rpm"`
	} `json:"idle"`
	Banks []struct {
		AngleDeg float64 `json:"angle_deg"`
	} `json:"banks"`
	Intakes []struct {
		RunnerLengthM float64 `json:"runner_length_m"`
	} `json:"intakes"`
	Exhaust []struct {
		LengthM         float64 `json:"length_m"`
		PrimaryTubeM    float64 `json:"primary_tube_length_m"`
		CollectorAreaM2 float64 `json:"collector_area_m2"`
		AudioVolume     float64 `json:"audio_volume"`
	} `json:"exhaust_systems"`
	Cylinders []struct {
		Bank           int     `json:"bank"`
		SparkDeg       float64 `json:"spark_deg"`
		SparkEnabled   bool    `json:"spark_enabled"`
		ExhaustSystem  int     `json:"exhaust_system"`
		Intake         int     `json:"intake"`
		HeaderPrimaryM float64 `json:"header_primary_length_m"`
	} `json:"cylinders"`
}

type naming struct{ id, description string }

// esNames names the engine-sim scripts. The identifiers and descriptions
// name no maker or model.
var esNames = map[string]naming{
	"atg-video-1/01_honda_trx520.mr":               {"es-single-atv", "Single cylinder, utility quad engine"},
	"atg-video-1/02_kohler_ch750.mr":               {"es-v2-90-industrial", "90 degree V-twin, air-cooled industrial engine"},
	"atg-video-1/03_harley_davidson_shovelhead.mr": {"es-v2-45-big-twin", "45 degree V-twin on a shared crank pin, uneven firing"},
	"atg-video-1/04_hayabusa.mr":                   {"es-i4-moto-litre", "Inline-4 motorcycle engine, 180 degree crank"},
	"atg-video-1/05_honda_vtec.mr":                 {"es-i4-dohc-cam-switch", "Inline-4, DOHC with a cam profile switch"},
	"atg-video-1/06_subaru_ej25.mr":                {"es-f4-boxer", "Flat-4 boxer, 2.5 litre"},
	"atg-video-1/07_audi_i5.mr":                    {"es-i5-23", "Inline-5, 2.3 litre"},
	"atg-video-1/08_radial_5.mr":                   {"es-r5-radial", "5-cylinder radial aero engine"},
	"atg-video-2/01_subaru_ej25_eh.mr":             {"es-f4-boxer-equal-header", "Flat-4 boxer, equal-length headers"},
	"atg-video-2/02_subaru_ej25_uh.mr":             {"es-f4-boxer-unequal-header", "Flat-4 boxer, unequal-length headers"},
	"atg-video-2/03_2jz.mr":                        {"es-i6-30", "Inline-6, 3.0 litre"},
	"atg-video-2/04_60_degree_v6.mr":               {"es-v6-60", "60 degree V6"},
	"atg-video-2/05_odd_fire_v6.mr":                {"es-v6-90-odd-fire", "90 degree V6 on common crank pins, odd firing"},
	"atg-video-2/06_even_fire_v6.mr":               {"es-v6-90-even-fire", "90 degree V6 on split crank pins, even firing"},
	"atg-video-2/07_gm_ls.mr":                      {"es-v8-crossplane-pushrod", "Cross-plane V8, pushrod"},
	"atg-video-2/08_ferrari_f136_v8.mr":            {"es-v8-flatplane-dohc", "Flat-plane V8, DOHC"},
	"atg-video-2/09_radial_9.mr":                   {"es-r9-radial", "9-cylinder radial aero engine"},
	"atg-video-2/10_lfa_v10.mr":                    {"es-v10-72", "72 degree V10, high-revving"},
	"atg-video-2/11_merlin_v12.mr":                 {"es-v12-60-aero", "60 degree V12 aero engine"},
	"atg-video-2/12_ferrari_412_t2.mr":             {"es-v12-75-race", "75 degree V12 race engine, uneven firing"},
	"audi/i5.mr":                                   {"es-i5-22", "Inline-5, 2.2 litre"},
	"bmw/M52B28.mr":                                {"es-i6-28-dohc", "Inline-6, 2.8 litre, DOHC"},
	"chevrolet/chev_truck_454.mr":                  {"es-v8-bigblock", "Cross-plane big-block V8, pushrod"},
	"chevrolet/engine_03_for_e1.mr":                {"es-v8-bigblock-alt", "Cross-plane big-block V8, pushrod, second tune"},
	"kohler/kohler_ch750.mr":                       {"es-v2-90-industrial-10k", "90 degree V-twin, air-cooled industrial engine, 10 kHz simulation"},
}

// loadEngineSim reads every dump under dir.
func loadEngineSim(dir string) ([]engine, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []engine
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(strings.TrimSuffix(rel, ".json")) + ".mr"
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var d esDump
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		e, err := fromEngineSim(rel, &d)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func fromEngineSim(rel string, d *esDump) (engine, error) {
	if len(d.Cylinders) == 0 {
		return engine{}, fmt.Errorf("no cylinders")
	}
	n := esNames[rel]
	e := engine{
		ID: n.id, Project: "engine-sim", File: rel, Description: n.description,
		Layout:      esLayout(d),
		RedlineRPM:  d.RedlineRPM,
		LimiterRPM:  d.RevLimitRPM,
		LimiterMS:   min(500, 1000*d.LimiterS),
		LimiterKind: "fuel",
		Crossover:   "none",
	}
	if d.LimiterS > 0.5 {
		e.Notes = append(e.Notes, fmt.Sprintf("Limiter duration %.0f ms capped at 500 ms.", 1000*d.LimiterS))
	}
	e.IdleRPM = math.Round(d.Idle.MeasuredRPM/10) * 10
	if d.Idle.MeasuredRPM < 300 {
		e.IdleRPM = fallbackIdle(d.RedlineRPM)
		e.Notes = append(e.Notes, fmt.Sprintf("Stalls at speed control 0, so idle_rpm is the fallback %.0f rpm.", e.IdleRPM))
	}

	// One snarl bank per exhaust system that a cylinder uses.
	keys := make([]int, len(d.Cylinders))
	for i, c := range d.Cylinders {
		keys[i] = c.ExhaustSystem
	}
	banks, systems := banksInOrder(keys)
	if len(systems) > 4 {
		return engine{}, fmt.Errorf("%d exhaust systems, snarl takes 4 banks", len(systems))
	}
	first := d.Cylinders[0].SparkDeg
	for i, c := range d.Cylinders {
		if !c.SparkEnabled {
			return engine{}, fmt.Errorf("cylinder %d has no ignition wire", i+1)
		}
		x := d.Exhaust[c.ExhaustSystem]
		e.Cylinders = append(e.Cylinders, cylinder{
			CrankDeg: wrap720(c.SparkDeg - first),
			Bank:     banks[i],
			PrimaryM: c.HeaderPrimaryM + x.PrimaryTubeM,
		})
	}
	loudest := 0.0
	for _, s := range systems {
		loudest = max(loudest, d.Exhaust[s].AudioVolume)
	}
	for _, s := range systems {
		x := d.Exhaust[s]
		r := 0.025
		if x.CollectorAreaM2 > 0 {
			r = math.Sqrt(x.CollectorAreaM2 / math.Pi)
		}
		length := x.LengthM
		if length <= 0 {
			length = 1
			e.Notes = append(e.Notes, "An exhaust system has no length, so collector_m is 1 m.")
		}
		gain := 1.0
		if loudest > 0 {
			gain = x.AudioVolume / loudest
		}
		e.Paths = append(e.Paths, exhaustPath{
			CollectorM: length, PipeRadiusM: r, OutletRadiusM: r,
			ChamberM: defaultChamberM, ChamberArea: defaultChamberArea, Gain: gain,
		})
	}
	var runner float64
	for _, c := range d.Cylinders {
		if c.Intake >= 0 && c.Intake < len(d.Intakes) {
			runner += d.Intakes[c.Intake].RunnerLengthM
		}
	}
	e.IntakeRunnerM = runner / float64(len(d.Cylinders))
	return e, nil
}

// esLayout describes the cylinder layout from the bank angles.
func esLayout(d *esDump) string {
	n := len(d.Cylinders)
	switch b := d.Banks; {
	case len(b) == 1 && n == 1:
		return "single"
	case len(b) == 1:
		return fmt.Sprintf("inline-%d", n)
	case len(b) == 2:
		a := math.Abs(b[1].AngleDeg - b[0].AngleDeg)
		if math.Abs(a-180) < 0.5 {
			return fmt.Sprintf("flat-%d", n)
		}
		return fmt.Sprintf("V%d, %.0f degree", n, a)
	default:
		return fmt.Sprintf("radial-%d", n)
	}
}

// fallbackIdle is the idle RPM of an engine that gives none: a fifth of
// the redline, rounded to 50 rpm, and at least 500 rpm.
func fallbackIdle(redline float64) float64 {
	return max(500, math.Round(0.2*redline/50)*50)
}

// wrap720 maps an angle in degrees to 0 up to 720, rounded to 0.001 degree.
func wrap720(deg float64) float64 {
	d := math.Mod(math.Round(deg*1000)/1000, 720)
	if d < 0 {
		d += 720
	}
	if d >= 720 {
		d = 0
	}
	return d
}
