package snarl

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// The reference tests compare each embedded profile with its own
// engine-sim reference renders: out/reference/upstream/<id> of just
// ref-upstream. The renders are not in the repository, so the tests that
// read them skip when they are not present and in short mode.

// upstreamScenario returns the path of the scenario name of profile id.
func upstreamScenario(id, name string) string {
	return filepath.Join("scenarios", "upstream", id, name+".scn")
}

// upstreamPath returns the path of the reference render of scenario name
// of profile id.
func upstreamPath(id, name string) string {
	return filepath.Join("out", "reference", "upstream", id, name+".wav")
}

// upstreamCache holds the reference renders at the test rate, by path.
var upstreamCache sync.Map

// upstreamRender returns the reference render of scenario name of profile
// id, mono at the test rate, and false when the render is not present or
// in short mode.
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
	x := a.Mono()
	if a.SampleRate != testRate {
		x = analysis.Resample(x, a.SampleRate, testRate)
	}
	upstreamCache.Store(path, x)
	return x, true
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
