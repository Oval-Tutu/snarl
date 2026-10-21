package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes text to dir/name.
func writeFile(t *testing.T, dir, name, text string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestUpstreamScenarios checks the scenarios of an engine with an idle of
// 812 RPM, a 6500 RPM redline and a 6500 RPM rev limit: the holds run from
// the idle rounded to 10 RPM to 100 RPM below the limiter, the free runs
// read back with their segments, and an engine that stalls takes the
// fallback idle.
func TestUpstreamScenarios(t *testing.T) {
	dir := t.TempDir()
	dump := writeFile(t, dir, "engine.json", `{"name": "Test", "simulation_frequency_hz": 20000, "redline_rpm": 6500, "rev_limit_rpm": 6500,
		"idle": {"measured_rpm": 812}, "cylinders": [{"spark_deg": 0}, {"spark_deg": 540}, {"spark_deg": 180}, {"spark_deg": 360}]}`)
	scn := filepath.Join(dir, "scn")
	var w bytes.Buffer
	if err := run([]string{"--upstream-scenarios", "--engine", dump, "--scenarios", scn, "--name", "f4-test"}, &w); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(scn, "holds-t050.scn"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hold 810 0.5 3\nsweep 810 2000 0.5 0.5\nhold 2000 0.5 3\n") || !strings.Contains(string(data), "hold 6400 0.5 3\n") {
		t.Errorf("holds-t050.scn:\n%s", data)
	}
	segs, err := readUpstreamSegments(filepath.Join(scn, "limiter-free.scn"))
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 || segs[0].kind != "hold" || segs[0].rpm != 5400 || !segs[0].nextFree || segs[1].kind != "free" || segs[1].throttle != 1 || segs[1].start != 0.5 || segs[1].end != 5.5 {
		t.Errorf("limiter-free segments %+v", segs)
	}
	data, err = os.ReadFile(filepath.Join(scn, "simrate-2x.scn"))
	if err != nil {
		t.Fatal(err)
	}
	// Three quarters of 6400 is 4800, nearest the 5000 RPM hold.
	if !strings.Contains(string(data), "40000 Hz") || !strings.Contains(string(data), "hold 5000 1 3\n") {
		t.Errorf("simrate-2x.scn:\n%s", data)
	}
	d, err := readUpstreamDump(dump)
	if err != nil {
		t.Fatal(err)
	}
	d.Idle.MeasuredRPM = 12
	if idle, stalled := d.idle(); idle != 1300 || !stalled {
		t.Errorf("stalled idle %g (%v), want the fallback 1300", idle, stalled)
	}
	order, iv := firingOrder(d.timeSpark())
	if joinInts(order, "-") != "1-3-4-2" || joinNums(iv, ",") != "180,180,180,180" {
		t.Errorf("firing order %v, intervals %v", order, iv)
	}
	// A negative starter speed turns the crank the other way, so that the
	// cylinders fire in falling spark angle.
	d.StarterRPM = -500
	if order, _ := firingOrder(d.timeSpark()); joinInts(order, "-") != "1-2-4-3" {
		t.Errorf("reversed firing order %v, want 1-2-4-3", order)
	}
}

// TestHealthMode checks the --health mode on a two-cylinder engine at 10
// kHz: a hold at 3000 RPM where both cylinders fire on their spark angles,
// and a free run at throttle 0 whose RPM wanders by 10 RPM at 2 Hz about
// 1000 RPM.
func TestHealthMode(t *testing.T) {
	dir := t.TempDir()
	dump := writeFile(t, dir, "engine.json", `{"name": "Twin", "simulation_frequency_hz": 10000, "redline_rpm": 4000, "rev_limit_rpm": 4200,
		"idle": {"measured_rpm": 1000}, "cylinders": [{"exhaust_system": 0, "spark_deg": 0}, {"exhaust_system": 0, "spark_deg": 360}]}`)
	writeFile(t, dir, "holds-t100.scn", "hold 3000 1 1.5\n")
	writeFile(t, dir, "idle-free.scn", "hold 1000 0 1\nfree 0 4\n")
	// Each cylinder fires every 40 ms at 3000 RPM, a pulse of 2 ms in its
	// runner and 0.3 of it in the other.
	var hold strings.Builder
	hold.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
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
		hold.WriteString(strings.Join([]string{ftoa(float64(s) / 10000), "3000", ftoa(101325 + p[0]), ftoa(101325 + p[1])}, ",") + "\n")
	}
	var free strings.Builder
	free.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
	for s := range 50000 {
		ts := float64(s) / 10000
		free.WriteString(strings.Join([]string{ftoa(ts), ftoa(1000 + 10*math.Sin(2*math.Pi*2*ts)), "101325", "101325"}, ",") + "\n")
	}
	var w bytes.Buffer
	err := run([]string{
		"--health", "--engine", dump, "--scenarios", dir, "--name", "twin",
		writeFile(t, dir, "holds-t100.csv", hold.String()), writeFile(t, dir, "idle-free.csv", free.String()),
	}, &w)
	if err != nil {
		t.Fatal(err)
	}
	out := w.String()
	for _, want := range []string{
		"Firing order 1-2, intervals 360, 360 degrees.",
		"| holds-t100 | 1 hold 3000 rpm, throttle 1 | 3000 | 1.8 |",
		"| 0 | 0 | 0.0 | 0.0 / 0.0 |",
		"| idle-free | 0 | 1000 | 7.1 | 990 | 1010 | 1000 | 2.00 / 10.0 | pass |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report has no %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| fail") {
		t.Errorf("report has a failed check:\n%s", out)
	}
}
