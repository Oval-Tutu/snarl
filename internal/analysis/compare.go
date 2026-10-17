package analysis

import (
	"fmt"
	"math"

	"github.com/oval-tutu/snarl/internal/scenario"
)

// Options controls a comparison.
type Options struct {
	// Orders are the engine orders for the order spectrum.
	Orders []float64
	// Window is the target length in seconds of each window along a sweep.
	Window float64
	// Settle is the time in seconds skipped at the start of each segment,
	// where the engine moves to the new state.
	Settle float64
	// FFTSizes are the multi-resolution STFT sizes.
	FFTSizes []int
}

// Window is one analysed part of the timeline.
type Window struct {
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	Line     int     `json:"scenario_line,omitempty"`
	Start    float64 `json:"start_s"`
	End      float64 `json:"end_s"`
	RPMStart float64 `json:"rpm_start,omitempty"`
	RPMEnd   float64 `json:"rpm_end,omitempty"`
	Throttle float64 `json:"throttle"`
	rpmAt    func(t float64) float64
}

// Windows splits a scenario into analysis windows: one per hold, and windows
// of about opts.Window seconds along each sweep. It drops the parts after
// duration seconds. With a nil scenario, it returns one window over the
// whole duration.
func Windows(sc *scenario.Scenario, duration float64, opts Options) []Window {
	if sc == nil {
		return []Window{{Label: "whole file", Kind: "file", End: duration}}
	}
	var out []Window
	start := 0.0
	for i, seg := range sc.Segments {
		segStart, segEnd := start, start+seg.Seconds
		start = segEnd
		s0 := segStart + min(opts.Settle, seg.Seconds/2)
		s1 := min(segEnd, duration)
		if s1-s0 <= 0 {
			continue
		}
		n := 1
		kind := "hold"
		if seg.Kind == scenario.Sweep {
			kind = "sweep"
			n = max(1, int(math.Round((s1-s0)/opts.Window)))
		}
		rpm := func(t float64) float64 {
			return seg.RPM0 + (seg.RPM1-seg.RPM0)*(t-segStart)/seg.Seconds
		}
		step := (s1 - s0) / float64(n)
		for j := range n {
			w := Window{
				Kind:     kind,
				Line:     seg.Line,
				Start:    s0 + float64(j)*step,
				End:      s0 + float64(j+1)*step,
				Throttle: seg.Throttle,
				rpmAt:    rpm,
			}
			w.RPMStart, w.RPMEnd = rpm(w.Start), rpm(w.End)
			w.Label = fmt.Sprintf("%d %s %.0f rpm", i+1, kind, w.RPMStart)
			if kind == "sweep" {
				w.Label = fmt.Sprintf("%d.%d sweep %.0f-%.0f rpm", i+1, j+1, w.RPMStart, w.RPMEnd)
			}
			out = append(out, w)
		}
	}
	return out
}

// Pair holds a metric for both inputs and their absolute difference.
type Pair struct {
	A    float64 `json:"a"`
	B    float64 `json:"b"`
	Diff float64 `json:"diff"`
}

func newPair(a, b float64) Pair {
	return Pair{A: a, B: b, Diff: math.Abs(a - b)}
}

// OrderMetrics is the order spectrum of both inputs in dBFS.
type OrderMetrics struct {
	A         []float64 `json:"a_db"`
	B         []float64 `json:"b_db"`
	Distance  float64   `json:"distance_db"`
	DominantA float64   `json:"dominant_a"`
	DominantB float64   `json:"dominant_b"`
}

// WindowMetrics is the result for one window.
type WindowMetrics struct {
	Window
	Order          *OrderMetrics `json:"order,omitempty"`
	ThirdOctaveLSD float64       `json:"third_octave_lsd_db"`
	MRSTFT         MRSTFTResult  `json:"mrstft"`
	CrestFactorDB  Pair          `json:"crest_factor_db"`
	SpectralFlux   Pair          `json:"spectral_flux"`
}

// Compare computes the metrics for each window. The inputs are mono at the
// same sample rate and already loudness-normalised. A is the reference for
// spectral convergence.
func Compare(a, b []float64, rate int, windows []Window, opts Options) []WindowMetrics {
	out := make([]WindowMetrics, 0, len(windows))
	n := min(len(a), len(b))
	for _, w := range windows {
		i0 := min(int(w.Start*float64(rate)), n)
		i1 := min(int(w.End*float64(rate)), n)
		xa, xb := a[i0:i1], b[i0:i1]
		m := WindowMetrics{
			Window:        w,
			MRSTFT:        MRSTFT(xa, xb, opts.FFTSizes),
			CrestFactorDB: newPair(CrestFactorDB(xa), CrestFactorDB(xb)),
			SpectralFlux:  newPair(SpectralFlux(xa), SpectralFlux(xb)),
		}
		_, la := ThirdOctaveLevels(xa, rate)
		_, lb := ThirdOctaveLevels(xb, rate)
		m.ThirdOctaveLSD = RMSDifferenceDB(la, lb)
		if w.rpmAt != nil {
			rpm := func(t float64) float64 { return w.rpmAt(w.Start + t) }
			oa, okA := OrderSpectrum(xa, rate, rpm, opts.Orders)
			ob, okB := OrderSpectrum(xb, rate, rpm, opts.Orders)
			if okA && okB {
				m.Order = &OrderMetrics{
					A:         oa,
					B:         ob,
					Distance:  RMSDifferenceDB(oa, ob),
					DominantA: DominantOrder(opts.Orders, oa),
					DominantB: DominantOrder(opts.Orders, ob),
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// Worst names the window with the largest value of one metric.
type Worst struct {
	Metric string  `json:"metric"`
	Label  string  `json:"label"`
	Index  int     `json:"index"`
	Value  float64 `json:"value"`
}

// MetricNames lists the metrics that WorstWindows ranks, in report order.
var MetricNames = []string{
	"order_distance_db",
	"third_octave_lsd_db",
	"mrstft_total",
	"crest_factor_diff_db",
	"spectral_flux_diff",
}

// MetricValue returns the named metric of m, and false when m lacks it.
func MetricValue(m *WindowMetrics, name string) (float64, bool) {
	switch name {
	case "order_distance_db":
		if m.Order == nil {
			return 0, false
		}
		return m.Order.Distance, true
	case "third_octave_lsd_db":
		return m.ThirdOctaveLSD, true
	case "mrstft_total":
		return m.MRSTFT.Total, true
	case "crest_factor_diff_db":
		return m.CrestFactorDB.Diff, true
	case "spectral_flux_diff":
		return m.SpectralFlux.Diff, true
	}
	return 0, false
}

// WorstWindows returns the window with the largest value of each metric.
// Metrics that no window has are left out.
func WorstWindows(ms []WindowMetrics) []Worst {
	var out []Worst
	for _, name := range MetricNames {
		w := Worst{Metric: name, Index: -1}
		for i := range ms {
			v, ok := MetricValue(&ms[i], name)
			if ok && (w.Index < 0 || v > w.Value) {
				w = Worst{Metric: name, Label: ms[i].Label, Index: i, Value: v}
			}
		}
		if w.Index >= 0 {
			out = append(out, w)
		}
	}
	return out
}
