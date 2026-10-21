package snarl

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

// alignTotal returns the alignment score of the render x against the
// reference ref over the windows of the scenario, as snarl-compare --align
// computes it, and the lowest window score with its label.
func alignTotal(ref, x []float64, scn *scenario.Scenario, cycles bool) (total, low float64, label string) {
	n := min(len(ref), len(x))
	windows := analysis.Windows(scn, float64(n)/testRate, analysis.Options{Settle: 0.25, Window: 0.5})
	var rs []analysis.AlignResult
	var weights []float64
	low = math.Inf(1)
	for _, w := range windows {
		i0, i1 := int(w.Start*testRate), min(int(w.End*testRate), n)
		r := analysis.Align(ref[i0:i1], x[i0:i1], testRate, w.RPMStart, w.Kind == "hold", cycles)
		rs = append(rs, r)
		weights = append(weights, float64(i1-i0))
		if r.Score < low {
			low, label = r.Score, w.Label
		}
	}
	total, _ = analysis.AlignTotal(rs, weights)
	return total, low, label
}

// TestScorecard renders the dynamometer scenarios of every profile, with
// the idle wander and the own pops off because engine-sim has neither,
// and checks that each scores at least 90 against its engine-sim reference
// with the alignment scorecard (docs/research/alignment-scorecard.md). It
// skips a scenario whose reference is not present, and runs only outside
// short mode.
func TestScorecard(t *testing.T) {
	if testing.Short() {
		t.Skip("the scorecard renders and scores every reference scenario")
	}
	t.Parallel()
	for _, id := range Profiles() {
		for _, name := range []string{"holds-t000", "holds-t050", "holds-t100", "sweep-wot", "overrun"} {
			t.Run(id+"/"+name, func(t *testing.T) {
				t.Parallel()
				ref, ok := upstreamRender(id, name)
				if !ok {
					t.Skipf("no reference %s", upstreamPath(id, name))
				}
				p, err := LoadProfileWith(id, "idle.hunt=0")
				if err != nil {
					t.Fatal(err)
				}
				x, scn := renderScenario(t, p, name, 3, FlagNoPops)
				total, low, label := alignTotal(ref, x, scn, true)
				msg := fmt.Sprintf("score %.1f, lowest window %s at %.1f", total, label, low)
				if total < 90 {
					t.Error(msg)
				} else {
					t.Log(msg)
				}
			})
		}
	}
}

// excerpt is a committed excerpt of an engine-sim reference: 0.6 s of the
// 3000 rpm full-throttle hold of out/reference/upstream/v8-ls/holds-t100.wav
// from 7.5 s, 0.5 s into the hold, at 48 kHz.
const excerpt = "v8-ls-holds-t100-3000.wav"

// TestScorecardExcerpt checks the engine against the committed excerpt in
// short mode too: the same hold, after the 3 s pre-roll of the reference
// and 0.5 s into the hold, scores at least 95 with every measure of the
// scorecard.
func TestScorecardExcerpt(t *testing.T) {
	a, err := wav.ReadFile(filepath.Join("testdata", "score", excerpt))
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != testRate {
		t.Fatalf("excerpt at %d Hz, want %d", a.SampleRate, testRate)
	}
	ref := a.Mono()
	p, err := LoadProfileWith("v8-ls", "idle.hunt=0")
	if err != nil {
		t.Fatal(err)
	}
	x := render(newEngine(t, p), 3.6+float64(len(ref))/testRate, held(3000, 1))
	b := make([]float64, len(ref))
	for i := range b {
		b[i] = float64(x[int(3.5*testRate)+i])
	}
	r := analysis.Align(ref, b, testRate, 3000, true, true)
	if r.Cycle == nil || r.Score < 95 {
		var s strings.Builder
		fmt.Fprintf(&s, "score %.1f, steps %+v", r.Score, r.Steps)
		if r.Cycle != nil {
			fmt.Fprintf(&s, ", wave error %.1f dB", r.Cycle.ErrorDB)
		}
		t.Error(s.String())
	}
}
