package cycles

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// ThrottleLag is the intake lag of the playback: after a throttle change,
// the weights of the throttle rows move to their new values with a first
// order lag of time constant Close (throttle closing) or Open (throttle
// opening), in seconds. 0 changes them at once.
type ThrottleLag struct {
	Close float64 `json:"close_s"`
	Open  float64 `json:"open_s"`
}

// StepSample is one cycle after a throttle step: its time from the step
// in seconds, and Alpha, the share of the state before the step that
// matches its cycle best: 1 before the step, 0 when the engine has
// settled at the new throttle.
type StepSample struct {
	Time, Alpha float64
	// Residual is the power of the cycle less the new state, over the
	// residual power of the new state in the tables: about 1 when the
	// engine has settled.
	Residual float64
	// Throttle is the throttle in steps of 0.01 whose table state, linear
	// between the throttle rows at the RPM, matches the cycle best, and
	// ThrottleResidual the residual left after that state, as Residual.
	Throttle, ThrottleResidual float64
	// RPM is the mean crank speed over the cycle.
	RPM float64
}

// findPoint returns the index of the table point at the RPM and throttle
// with the ignition on, or -1.
func findPoint(t *Tables, rpm, throttle float64) int {
	for i, p := range t.Points {
		if p.Ignition && p.Since == 0 && p.RPM == rpm && p.Throttle == throttle {
			return i
		}
	}
	return -1
}

// StepResponse measures the throttle step into segment k, a hold that
// follows a hold at the same RPM with another throttle. Each whole cycle
// of segment k is resampled as in Measure and projected onto the table
// points of the two throttles at that RPM, channel by channel, over the
// harmonics without the mean.
func (r *Record) StepResponse(t *Tables, k int, opt MeasureOptions) ([]StepSample, error) {
	if k < 1 || k >= len(r.Segments) {
		return nil, errors.New("no segment before the step")
	}
	prev, seg := r.Segments[k-1], r.Segments[k]
	if prev.Mode != "hold" || seg.Mode != "hold" || prev.RPM0 != seg.RPM0 || prev.Throttle == seg.Throttle {
		return nil, fmt.Errorf("segment %d is not a throttle step between holds at one RPM", k)
	}
	a, b := findPoint(t, seg.RPM0, prev.Throttle), findPoint(t, seg.RPM0, seg.Throttle)
	if a < 0 || b < 0 {
		return nil, fmt.Errorf("the tables have no points at %g rpm and throttle %g and %g", seg.RPM0, prev.Throttle, seg.Throttle)
	}
	pa, pb := &t.Points[a], &t.Points[b]
	h := min(pa.Harmonics, pb.Harmonics)
	s, err := r.HoldSpan(seg, 0)
	if err != nil {
		return nil, err
	}
	delays := r.ChannelDelays()
	stepsPerCycle := float64(s.I1-s.I0) / float64(s.Cycles)
	// The harmonics above the Nyquist frequency of the steps are not in a
	// synchronous average of one cycle.
	h = min(h, int(math.Floor(stepsPerCycle/2)))
	pad := int(math.Ceil(maxOf(delays))) + sincHalf + 2
	smooth := r.smoothedPhase(s, pad, stepsPerCycle, opt)
	stepTime := seg.Start
	var out []StepSample
	var steady float64
	for c := range r.InputChannelCount {
		steady += NoisePower(pb.Noise[c])
	}
	// The throttle rows at the RPM, for the best-matching throttle.
	var rows []int
	for i, p := range t.Points {
		if p.Ignition && p.Since == 0 && p.RPM == seg.RPM0 {
			rows = append(rows, i)
			h = min(h, p.Harmonics)
		}
	}
	slices.SortFunc(rows, func(a, b int) int { return cmpFloat(t.Points[a].Throttle, t.Points[b].Throttle) })
	for n := range s.Cycles {
		var num, den, mid, res, cycleRPM float64
		dist := make([]float64, 101)
		for c := range r.InputChannelCount {
			d := int(math.Round(delays[c]))
			lo, hi := s.I0-sincHalf-2, min(s.I1+sincHalf+2, len(r.Inputs[c]))
			ph := smooth.slice(lo-d, hi-d)
			start := math.Ceil(ph[sincHalf+2]) + float64(n)
			if start+1 > ph[len(ph)-1] {
				continue
			}
			y := syncAverage(r.Inputs[c][lo:hi], ph, start, 1, h, 0)
			for j := 1; j < 2*h+1; j++ {
				d0 := y[j] - pb.Wave[c][j]
				d1 := pa.Wave[c][j] - pb.Wave[c][j]
				num += d0 * d1
				den += d1 * d1
				// The series power counts each harmonic twice.
				res += 2 * d0 * d0
			}
			for k := range dist {
				th := float64(k) / 100
				i0, i1, f := bracket(len(rows), func(i int) float64 { return t.Points[rows[i]].Throttle }, th)
				u0, u1 := t.Points[rows[i0]].Wave[c], t.Points[rows[i1]].Wave[c]
				for j := 1; j < 2*h+1; j++ {
					d := y[j] - ((1-f)*u0[j] + f*u1[j])
					dist[k] += 2 * d * d
				}
			}
			// The time of the middle of the cycle and its mean speed, from
			// the channel 0 phase.
			if c == 0 {
				at := func(psi float64) float64 {
					k := 0
					for k+1 < len(ph) && ph[k+1] < psi {
						k++
					}
					f := 0.0
					if dp := ph[k+1] - ph[k]; dp > 0 {
						f = (psi - ph[k]) / dp
					}
					return (float64(lo+k+1) + f) * r.Timestep
				}
				t0, t1 := at(start), at(start+1)
				mid = (t0+t1)/2 - stepTime
				cycleRPM = 120 / (t1 - t0)
			}
		}
		if den > 0 {
			best := 0
			for k, d := range dist {
				if d < dist[best] {
					best = k
				}
			}
			out = append(out, StepSample{
				Time: mid, Alpha: num / den, Residual: res / steady,
				Throttle: float64(best) / 100, ThrottleResidual: dist[best] / steady, RPM: cycleRPM,
			})
		}
	}
	return out, nil
}

// NoisePower returns the mean residual power of a Noise table over the
// sectors, summed over the bands.
func NoisePower(e []float64) float64 {
	var p float64
	for _, v := range e {
		p += v * v
	}
	return p / Sectors
}

func maxOf(x []float64) float64 {
	m := math.Inf(-1)
	for _, v := range x {
		m = max(m, v)
	}
	return m
}

// FitExcess fits the residual after a step with 1 + a exp(-t/tau): the
// excess residual power a at the step over the settled residual, and its
// time constant tau, on a logarithmic grid from 5 ms to 5 s. a is not
// negative.
func FitExcess(samples []StepSample) (a, tau float64) {
	best := math.Inf(1)
	for i := range 601 {
		c := 0.005 * math.Pow(1000, float64(i)/600)
		var eg, gg float64
		for _, s := range samples {
			g := math.Exp(-max(s.Time, 0) / c)
			eg += (s.Residual - 1) * g
			gg += g * g
		}
		if gg == 0 {
			continue
		}
		k := max(eg/gg, 0)
		var e float64
		for _, s := range samples {
			d := s.Residual - 1 - k*math.Exp(-max(s.Time, 0)/c)
			e += d * d
		}
		if e < best {
			best, a, tau = e, k, c
		}
	}
	return a, tau
}

// FitLag returns the time constant tau of exp(-t/tau) that fits the step
// samples best in least squares, on a logarithmic grid from 5 ms to 5 s,
// and the RMS error of the fit.
func FitLag(samples []StepSample) (tau, rms float64) {
	best := math.Inf(1)
	for i := range 601 {
		c := 0.005 * math.Pow(1000, float64(i)/600)
		var e float64
		for _, s := range samples {
			d := s.Alpha - math.Exp(-max(s.Time, 0)/c)
			e += d * d
		}
		if e < best {
			best, tau = e, c
		}
	}
	return tau, math.Sqrt(best / float64(max(len(samples), 1)))
}
