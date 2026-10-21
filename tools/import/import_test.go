package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestEngineLabGraph checks the bank grouping, primaries, collectors,
// chamber and crossover of a two-bank graph with an X-pipe, and the even
// crank offsets of a firing order.
func TestEngineLabGraph(t *testing.T) {
	const src = `{"engine": {"layout": "v", "bank_angle_deg": 90, "cylinder_count": 4, "firing_order": [1, 4, 2, 3],
	"idle_rpm": 700, "redline_rpm": 6000, "ignition": {"rev_limit_rpm": 6100, "limiter_duration_s": 0.06},
	"exhaust_paths": [{"id": 1, "cylinder_ids": [1, 2, 3, 4], "graph": {
		"components": [
			{"id": 1, "type": "pipe", "length_mm": 600, "diameter_mm": 40},
			{"id": 2, "type": "pipe", "length_mm": 600, "diameter_mm": 40},
			{"id": 3, "type": "pipe", "length_mm": 600, "diameter_mm": 40},
			{"id": 4, "type": "pipe", "length_mm": 600, "diameter_mm": 40},
			{"id": 11, "type": "merge", "length_mm": 500, "diameter_mm": 60, "volume_l": 2},
			{"id": 12, "type": "merge", "length_mm": 500, "diameter_mm": 60, "volume_l": 2},
			{"id": 20, "type": "crossover", "crossover_coupling": 0.7},
			{"id": 21, "type": "muffler", "length_mm": 400, "diameter_mm": 60, "volume_l": 5.6},
			{"id": 22, "type": "muffler", "length_mm": 400, "diameter_mm": 60, "volume_l": 5.6},
			{"id": 31, "type": "outlet", "length_mm": 100, "diameter_mm": 70},
			{"id": 32, "type": "outlet", "length_mm": 100, "diameter_mm": 70}],
		"cylinder_connections": [{"cylinder_id": 1, "to_component_id": 1}, {"cylinder_id": 2, "to_component_id": 2},
			{"cylinder_id": 3, "to_component_id": 3}, {"cylinder_id": 4, "to_component_id": 4}],
		"connections": [{"from_component_id": 1, "to_component_id": 11}, {"from_component_id": 3, "to_component_id": 11},
			{"from_component_id": 2, "to_component_id": 12}, {"from_component_id": 4, "to_component_id": 12},
			{"from_component_id": 11, "to_component_id": 20, "to_port": 0}, {"from_component_id": 12, "to_component_id": 20, "to_port": 1},
			{"from_component_id": 20, "from_port": 0, "to_component_id": 21}, {"from_component_id": 20, "from_port": 1, "to_component_id": 22},
			{"from_component_id": 21, "to_component_id": 31}, {"from_component_id": 22, "to_component_id": 32}]}}]}}`
	var d elFile
	if err := json.Unmarshal([]byte(src), &d); err != nil {
		t.Fatal(err)
	}
	e, err := fromEngineLab("test.engine.yaml", &d.Engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantDeg := []float64{0, 360, 540, 180}
	wantBank := []int{0, 1, 0, 1}
	for i, c := range e.Cylinders {
		if c.CrankDeg != wantDeg[i] || c.Bank != wantBank[i] || c.PrimaryM != 0.6 {
			t.Errorf("cylinder %d: %+v, want crank %v, bank %d, primary 0.6", i+1, c, wantDeg[i], wantBank[i])
		}
	}
	if e.Crossover != "partial" || e.Coupling != 0.5 || len(e.Paths) != 2 {
		t.Fatalf("crossover %s %v with %d paths, want partial 0.5 with 2", e.Crossover, e.Coupling, len(e.Paths))
	}
	// Collector: merge 0.5 m and outlet 0.1 m. Chamber: 5.6 L over 0.4 m
	// is 14000 mm2, 4.95 times the 60 mm pipe.
	p := e.Paths[0]
	if round(p.CollectorM, 6) != 0.6 || p.PipeRadiusM != 0.03 || p.OutletRadiusM != 0.035 || p.ChamberM != 0.4 || p.ChamberArea != 4.95 {
		t.Errorf("path %+v", p)
	}
	if e.LimiterRPM != 6100 || e.LimiterMS != 60 {
		t.Errorf("limiter %v rpm, %v ms", e.LimiterRPM, e.LimiterMS)
	}
}

// TestEngineSimDump checks the crank offsets from the spark angles, the
// banks from the exhaust systems and the idle fallback of a stalled engine.
func TestEngineSimDump(t *testing.T) {
	const src = `{"redline_rpm": 6000, "rev_limit_rpm": 6500, "limiter_duration_s": 0.8, "idle": {"measured_rpm": 0.001},
	"hf_gain": 0.002, "noise": 0.25,
	"banks": [{"angle_deg": -45}, {"angle_deg": 45}], "intakes": [{"runner_length_m": 0.25}],
	"exhaust_systems": [{"length_m": 2, "primary_tube_length_m": 0.5, "collector_area_m2": 0.002, "audio_volume": 2, "impulse_response": "a.wav"},
		{"length_m": 3, "primary_tube_length_m": 0.5, "collector_area_m2": 0.002, "audio_volume": 1, "impulse_response": "a.wav"}],
	"cylinders": [
		{"bank": 0, "spark_deg": 90, "spark_enabled": true, "exhaust_system": 1, "intake": 0, "header_primary_length_m": 0.1},
		{"bank": 1, "spark_deg": 450, "spark_enabled": true, "exhaust_system": 0, "intake": 0, "header_primary_length_m": 0.2},
		{"bank": 0, "spark_deg": 0, "spark_enabled": true, "exhaust_system": 1, "intake": 0, "header_primary_length_m": 0}]}`
	var d esDump
	if err := json.Unmarshal([]byte(src), &d); err != nil {
		t.Fatal(err)
	}
	e, err := fromEngineSim("test.mr", &d)
	if err != nil {
		t.Fatal(err)
	}
	want := []cylinder{{0, 0, 0.6}, {360, 1, 0.7}, {630, 0, 0.5}}
	for i, c := range e.Cylinders {
		if c != want[i] {
			t.Errorf("cylinder %d: %+v, want %+v", i+1, c, want[i])
		}
	}
	if len(e.Paths) != 2 || e.Paths[0].CollectorM != 3 || e.Paths[0].Gain != 0.5 || e.Paths[1].Gain != 1 {
		t.Errorf("paths %+v", e.Paths)
	}
	if e.IdleRPM != 1200 || e.LimiterMS != 500 || e.Layout != "V3, 90 degree" {
		t.Errorf("idle %v, limiter %v ms, layout %q", e.IdleRPM, e.LimiterMS, e.Layout)
	}
	// The jitter is missing from the dump, so it has the engine-sim default.
	if e.HFGain != 0.002 || e.AM != 0.25 || e.InputJitter != defaultJitter || e.ImpulseResponse != "a.wav" {
		t.Errorf("hf_gain %v, am %v, input_jitter %v, impulse response %q", e.HFGain, e.AM, e.InputJitter, e.ImpulseResponse)
	}
}

// TestPopKey checks that the pop bank follows the pop stages of the first
// path, that paths with the same stages share them, and that other stages
// give a note.
func TestPopKey(t *testing.T) {
	e := engine{Paths: []exhaustPath{
		{CollectorM: 1, PipeRadiusM: 0.03, OutletRadiusM: 0.03, ChamberM: 0.3, ChamberArea: 7, Gain: 1},
		{CollectorM: 2, PipeRadiusM: 0.03, OutletRadiusM: 0.03, ChamberM: 0.3, ChamberArea: 7, Gain: 0.5},
	}}
	k, note := popKey(&e)
	if note != "" || k.PipeRadiusM != 0.03 || k.ChamberM != 0.3 || k.CollectorM != 0 || k.Gain != 0 || k.WallLossDB != legacyWallLossDB {
		t.Errorf("key %+v, note %q", k, note)
	}
	e.Paths[1].ChamberM = 0.4
	if _, note := popKey(&e); note == "" {
		t.Error("paths with other chambers gave no note")
	}
}

// TestFitBanksMissingIR checks that fitBanks returns an error for each
// missing impulse response. The fits that fail fast write their results
// while the other fits start, so this test also finds a race on the maps
// of fitBanks under -race.
func TestFitBanksMissingIR(t *testing.T) {
	const n = 64
	all := make([]engine, n)
	for i := range all {
		all[i] = engine{
			ImpulseResponse: fmt.Sprintf("missing-%d.wav", i),
			Paths:           []exhaustPath{{PipeRadiusM: 0.03, OutletRadiusM: 0.03, ChamberM: 0.3, ChamberArea: 7, Gain: 1}},
		}
	}
	err := fitBanks(all, t.TempDir())
	if err == nil {
		t.Fatal("fitBanks gave no error for missing impulse responses")
	}
	for i := range all {
		if want := fmt.Sprintf("no impulse response missing-%d.wav", i); !strings.Contains(err.Error(), want) {
			t.Errorf("the error has no %q", want)
		}
	}
}

// TestPulseFor checks that every cylinder count takes the pulse table and
// the collector overlap of the tuned profile closest in layout.
func TestPulseFor(t *testing.T) {
	for n, want := range map[int]string{1: "inline4", 4: "inline4", 5: "inline4", 6: "flat6", 8: "v8", 9: "v8", 10: "flat12", 16: "flat12"} {
		table, overlap := pulseFor(n)
		if table != want || (table == "inline4") != (overlap == 0) {
			t.Errorf("%d cylinders: table %s and overlap %g, want %s with the overlap of that profile", n, table, overlap, want)
		}
	}
}
