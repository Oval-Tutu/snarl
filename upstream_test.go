package snarl

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// The reference tests compare each embedded profile with the measures of
// its own upstream reference renders: out/reference/upstream/<id> of just
// ref-upstream for the engine-sim engines, and
// out/reference/enginelab/f6-aircooled of just ref-enginelab. The renders
// are not in the repository, so each test holds the measures of the
// references in a table. When the renders are present, the test measures
// them again and fails when a recorded measure is more than 0.05 from the
// render, so that every recorded value stays traceable to its render.

// upstreamScenario returns the path of the scenario name of profile id.
func upstreamScenario(id, name string) string {
	return filepath.Join("scenarios", "upstream", id, name+".scn")
}

// upstreamPath returns the path of the reference render of scenario name
// of profile id.
func upstreamPath(id, name string) string {
	if id == "f6-aircooled" {
		return filepath.Join("out", "reference", "enginelab", id, name+".wav")
	}
	return filepath.Join("out", "reference", "upstream", id, name+".wav")
}

// upstreamCache holds the reference renders at the test rate, by path.
var upstreamCache sync.Map

// upstreamRender returns the reference render of scenario name of profile
// id, mono at the test rate, and false when the render is not present or
// in short mode, which leaves out the check of the recorded measures.
func upstreamRender(id, name string) ([]float64, bool) {
	if testing.Short() {
		return nil, false
	}
	path := upstreamPath(id, name)
	if v, ok := upstreamCache.Load(path); ok {
		return v.([]float64), true
	}
	a, err := wav.ReadFile(path)
	if err != nil {
		return nil, false
	}
	x := analysis.Resample(a.Mono(), a.SampleRate, testRate)
	upstreamCache.Store(path, x)
	return x, true
}

// checkRecorded logs the measures of a reference render and reports an
// error when one is more than 0.05 from its recorded value.
func checkRecorded(t *testing.T, what string, got, recorded []float64) {
	t.Helper()
	s := make([]string, len(got))
	for i, v := range got {
		s[i] = fmt.Sprintf("%.2f", v)
	}
	t.Logf("%s: reference %s: {%s}", t.Name(), what, strings.Join(s, ", "))
	if len(got) != len(recorded) {
		t.Errorf("reference %s: %d measures, %d recorded", what, len(got), len(recorded))
		return
	}
	for i := range got {
		if math.Abs(got[i]-recorded[i]) > 0.05 {
			t.Errorf("reference %s: measure %d is %.2f, recorded %.2f", what, i+1, got[i], recorded[i])
		}
	}
}

// parseUpstream parses the scenario name of profile id.
func parseUpstream(t *testing.T, id, name string) *scenario.Scenario {
	t.Helper()
	scn, err := scenario.ParseFile(upstreamScenario(id, name))
	if err != nil {
		t.Fatal(err)
	}
	return scn
}
