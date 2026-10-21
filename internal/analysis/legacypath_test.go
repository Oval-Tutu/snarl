package analysis

import (
	"math"
	"strings"
	"testing"
)

// TestReadLegacyExhaust checks that the reader takes the exhaust and path
// keys over the defaults, skips other tables and rejects an unknown path
// key.
func TestReadLegacyExhaust(t *testing.T) {
	src := `name = "x"
[pulse]
width_deg = 170
[exhaust]
crossover = "partial" # a string key
low_shelf_db = -3
[[path]]
collector_m = 1.10
reflection = -0.45
[[path]]
collector_m = 1.28
gain = 0.7
[voice]
output_db = -1.5
`
	e, paths, err := ReadLegacyExhaust(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if e.LowShelfDB != -3 || e.SpeedOfSound != defaultLegacyExhaust().SpeedOfSound {
		t.Errorf("exhaust = %+v", e)
	}
	if len(paths) != 2 || paths[0].CollectorM != 1.10 || paths[0].Reflection != -0.45 || paths[0].Gain != 1 || paths[1].Gain != 0.7 {
		t.Errorf("paths = %+v", paths)
	}
	if _, _, err := ReadLegacyExhaust(strings.NewReader("[[path]]\nlength = 2\n")); err == nil {
		t.Error("an unknown path key gave no error")
	}
}

// TestLegacyCollectorComb checks that the collector alone, with a
// negative reflection, peaks at the odd multiples of c/(4L) and dips at
// the even multiples.
func TestLegacyCollectorComb(t *testing.T) {
	e := defaultLegacyExhaust()
	p := LegacyPath{CollectorM: 1, Reflection: -0.5}
	h := LegacyPathImpulse(&p, &e, 48000, 1<<16, LegacyOnlyStage(StageCollector))
	f := e.SpeedOfSound / 4
	band := func(c float64) [2]float64 { return [2]float64{c * 0.98, c * 1.02} }
	r := IRResponse(h, 48000, [][2]float64{band(f), band(2 * f)})
	// 1/(1-0.5) against 1/(1+0.5): 20 log10(3) = 9.5 dB.
	if d := r[0] - r[1]; math.Abs(d-9.5) > 1 {
		t.Errorf("peak to dip = %.2f dB, want about 9.5 dB", d)
	}
}

// TestLegacyNoStage checks that a path with no stage selected passes the
// impulse unchanged.
func TestLegacyNoStage(t *testing.T) {
	e := defaultLegacyExhaust()
	p := LegacyPath{CollectorM: 1, Reflection: -0.5, PipeRadiusM: 0.03, OutletM: 0.03}
	h := LegacyPathImpulse(&p, &e, 48000, 8, [LegacyStageCount]bool{})
	if h[0] != 1 || h[1] != 0 {
		t.Errorf("impulse response = %v, want a unit impulse", h)
	}
}
