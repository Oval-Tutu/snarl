package main

import (
	"math"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// TestDriveWindows checks the windows of a short drive: idle, a pull with
// a gear change, a lift and a coast, at idle 1000 and limiter 7000 rpm.
func TestDriveWindows(t *testing.T) {
	sc, err := scenario.Parse(strings.NewReader(`
hold 1000 0 0.5
sweep 1000 3000 1 0.5
sweep 3000 4000 1 0.24
sweep 4000 3000 1 0.01
sweep 3000 3100 1 0.25
sweep 3100 2900 0 0.25
sweep 2900 2700 0 0.25
`))
	if err != nil {
		t.Fatal(err)
	}
	ws, ds := driveWindows(sc, 0.25, sc.Duration(), 1000, 7000)
	want := []string{"idle", "idle", markTipIn, "full-throttle pull", markShift, "full-throttle pull", markLift, "coast"}
	if len(ws) != len(want) {
		t.Fatalf("%d windows, want %d", len(ws), len(want))
	}
	for i := range ws {
		if got := driveClass(&ds[i]); got != want[i] {
			t.Errorf("window %d (%s): class %q, want %q", i, ws[i].Label, got, want[i])
		}
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	if d := ds[4]; !near(d.hi, 4000) || !near(d.lo, 3000) || !near(d.rpm0, 3000) || !near(d.rpm1, 3000) {
		t.Errorf("shift window RPM %+v", d)
	}
	if d := ds[6]; d.th0 != 1 || d.th1 != 0 || !near(d.rate(ws[6]), -800) {
		t.Errorf("lift window %+v, rate %v", d, d.rate(ws[6]))
	}
}

// TestDriveMisses checks the miss marks against the floors and against the
// largest values of engine-sim against itself.
func TestDriveMisses(t *testing.T) {
	window := func(hiss, tonal float64) *point {
		return &point{r: analysis.AlignResult{
			Hiss:      analysis.HissMatch{WorstDB: hiss},
			HissCycle: &analysis.HissCycleMatch{TonalA: []float64{10}, TonalB: []float64{10 + tonal}, GapA: []float64{-20}, GapB: []float64{-20}},
		}}
	}
	self := driveSelf([]*point{window(2.5, 0.25), window(-3, 0.125)})
	if self != [3]float64{3, 0.25, 0} {
		t.Errorf("self %v", self)
	}
	for _, c := range []struct {
		hiss, tonal float64
		want        string
	}{
		{1, 0.1, ""},
		{-2, 0.1, markHiss},
		{4, -0.6, markHiss + markBeyondSelf + markTonality + markBeyondSelf},
	} {
		if got := driveMisses(window(c.hiss, c.tonal), self); got != c.want {
			t.Errorf("band %v, tonality %v: marks %q, want %q", c.hiss, c.tonal, got, c.want)
		}
	}
}
