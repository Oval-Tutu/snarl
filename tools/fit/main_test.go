package main

import (
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

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
