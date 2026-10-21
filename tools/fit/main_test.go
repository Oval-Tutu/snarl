package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// TestPulseMode checks the --pulse mode on a small engine dump, scenario
// and CSV: the engine dump gives the exhaust system and the spark angle of
// each cylinder, the report names every hold, and the pulse file has one
// row per RPM with the RPM, the length and the values.
func TestPulseMode(t *testing.T) {
	dir := t.TempDir()
	dump := `{"cylinders": [{"exhaust_system": 0, "spark_deg": 0}, {"exhaust_system": 0, "spark_deg": 360}]}`
	if err := os.WriteFile(filepath.Join(dir, "engine.json"), []byte(dump), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holds-t100.scn"), []byte("hold 3000 1 1.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 1.5 s at 10 kHz and 3000 RPM: each cylinder fires every 40 ms, a
	// pulse of 2 ms in its runner and 0.3 of it in the other.
	var csv strings.Builder
	csv.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
	for s := range 15000 {
		var p [2]float64
		for c := range p {
			ms := float64((s-c*200)%400) / 10
			if ms >= 0 && ms < 2 {
				v := 1000 * (ms / 2) * (2 - ms)
				p[c] += v
				p[1-c] += 0.3 * v
			}
		}
		csv.WriteString(strings.Join([]string{ftoa(float64(s) / 10000), "3000", ftoa(101325 + p[0]), ftoa(101325 + p[1])}, ",") + "\n")
	}
	path := filepath.Join(dir, "holds-t100.csv")
	if err := os.WriteFile(path, []byte(csv.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "pulse.txt")
	var w bytes.Buffer
	err := run([]string{
		"--pulse", "--throttle", "1", "--points", "32", "--engine", filepath.Join(dir, "engine.json"),
		"--scenarios", dir, "--out", out, path,
	}, &w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), "holds-t100 1 hold 3000 rpm") {
		t.Errorf("report names no hold:\n%s", w.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "row ") {
			rows = append(rows, line)
		}
	}
	if len(rows) != 1 || len(strings.Fields(rows[0])) != 3+32 || !strings.HasPrefix(rows[0], "row 3000 ") {
		t.Errorf("pulse file rows %q, want one row of 32 values at 3000 RPM", rows)
	}
	if err := run([]string{"--pulse", path}, &w); err == nil {
		t.Error("--pulse with no engine dump gave no error")
	}
}

func ftoa(x float64) string {
	return strconv.FormatFloat(x, 'f', 6, 64)
}

// TestPulseVariationMode checks --pulse --variation on a CSV whose first
// cylinder fires at 0.9 and 1.1 of the pulse in turn and whose second
// cylinder fires at 1: the report has the variation table, and the pulse
// file has one spread line per row with the gain spread about the mean of
// each cylinder, sqrt(0.1^2/2) = 0.071 relative to the mean gain of 1.
func TestPulseVariationMode(t *testing.T) {
	dir := t.TempDir()
	dump := `{"cylinders": [{"exhaust_system": 0, "spark_deg": 0}, {"exhaust_system": 0, "spark_deg": 360}]}`
	if err := os.WriteFile(filepath.Join(dir, "engine.json"), []byte(dump), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holds-t100.scn"), []byte("hold 3000 1 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var csv strings.Builder
	csv.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
	for s := range 20000 {
		var p [2]float64
		for c := range p {
			k := (s - c*200) / 400
			ms := float64((s-c*200)%400) / 10
			g := 1.0
			if c == 0 {
				g = 0.9 + 0.2*float64(k%2)
			}
			if s >= c*200 && ms < 2 {
				v := 1000 * g * (ms / 2) * (2 - ms)
				p[c] += v
				p[1-c] += 0.3 * v
			}
		}
		csv.WriteString(strings.Join([]string{ftoa(float64(s) / 10000), "3000", ftoa(101325 + p[0]), ftoa(101325 + p[1])}, ",") + "\n")
	}
	path := filepath.Join(dir, "holds-t100.csv")
	if err := os.WriteFile(path, []byte(csv.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "pulse.txt")
	var w bytes.Buffer
	err := run([]string{
		"--pulse", "--variation", "--throttle", "1", "--points", "32", "--engine", filepath.Join(dir, "engine.json"),
		"--scenarios", dir, "--out", out, path,
	}, &w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), "Cycle-to-cycle variation") {
		t.Errorf("report has no variation table:\n%s", w.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var spreads []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "spread ") {
			spreads = append(spreads, line)
		}
	}
	if len(spreads) != 1 {
		t.Fatalf("spread lines %q, want one", spreads)
	}
	f := strings.Fields(spreads[0])
	g, err := strconv.ParseFloat(f[len(f)-1], 64)
	if err != nil || len(f) != 3 || f[1] != "3000" || g < 0.06 || g > 0.08 {
		t.Errorf("spread line %q, want spread 3000 and a gain spread of 0.071 within 0.01", spreads[0])
	}
}

// TestPopTarget checks that the pop target leaves out the collector, that
// paths may differ in their collector, gain and listener distance, and
// that a path with other pop stage keys is an error.
func TestPopTarget(t *testing.T) {
	e := analysis.LegacyExhaust{SpeedOfSound: 565, OutletDirect: 0.4, LowShelfHz: 250}
	a := analysis.LegacyPath{
		CollectorM: 1.1, Reflection: -0.45, DampingHz: 2200, PipeRadiusM: 0.0286, OutletM: 0.0286,
		WallLossDB: 3, WallLossHz: 1800, ChamberM: 0.35, ChamberArea: 8, Gain: 1,
	}
	b := a
	b.CollectorM, b.Gain, b.ListenerM = 1.28, 0.7, 0.3
	cfg := config{pop: true, profile: "x.toml", rate: 48000}
	h, rate, _, err := loadTarget(&cfg, []analysis.LegacyPath{a, b}, &e)
	if err != nil {
		t.Fatal(err)
	}
	want := analysis.LegacyPathImpulse(&b, &e, 48000, pathTaps, analysis.LegacyPopStages())
	if rate != 48000 || len(h) != len(want) || h[100] != want[100] {
		t.Errorf("the pop target depends on the collector of the path")
	}
	if d := analysis.LegacyPopDelayMS(&a, &e); d < 0.61 || d > 0.63 {
		t.Errorf("pop delay %.3f ms, want 0.35 m / 565 m/s = 0.619 ms", d)
	}
	b.WallLossDB = 2
	if _, _, _, err := loadTarget(&cfg, []analysis.LegacyPath{a, b}, &e); err == nil {
		t.Error("paths with other wall loss gave no error")
	}
	if !(&config{base: "harmonic"}).harmonic() || (&config{base: "ir"}).harmonic() || (&config{base: "harmonic", pop: true}).harmonic() {
		t.Error("harmonic() does not follow --base and --pop")
	}
}
