package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

const (
	// levelRate is the synthesiser rate. At this output rate the engine has
	// no resampler, so that a render with pops less the same render with
	// no pops is the pops after the pop limiter, sample for sample.
	levelRate = 44100
	// levelBlock is the render block, as in snarl-render.
	levelBlock = 256
	// overrunThrottle is the throttle below which the snapshot is on
	// overrun, as in snarl-render.
	overrunThrottle = 0.05
	// momentaryS is the window of the momentary loudness of ITU-R BS.1770.
	momentaryS = 0.4
	// popSpanS is the longest span of one pop, when no later pop ends it.
	popSpanS = 1
)

// levelSets are the profile overrides of the pop level measures and of
// the level variants: the race character, as in just listen-pops-model.
var levelSets = []string{`pops.character="race"`}

// popKindNames are the profile names of the pop kinds, in the order of the
// snarl PopKind values from KindPop.
var popKindNames = []string{"pop", "boom", "crackle", "burble"}

// levelCase is one engine and its lift-off scenario.
type levelCase struct {
	profile, scenario string
}

// parseCases returns the cases of the arguments PROFILE=SCENARIO.
func parseCases(args []string) ([]levelCase, error) {
	var cs []levelCase
	for _, a := range args {
		p, s, ok := strings.Cut(a, "=")
		if !ok || p == "" || s == "" {
			return nil, fmt.Errorf("argument %q: want PROFILE=SCENARIO", a)
		}
		cs = append(cs, levelCase{p, s})
	}
	return cs, nil
}

// renderEngine renders the scenario of c on the profile with the overrides
// at the rate, as snarl-render does. With noPops the snapshots set
// FlagNoPops. It returns the render and the start sample of each pop.
func renderEngine(c levelCase, sets []string, noPops bool, rate int) ([]float64, []int, error) {
	p, err := snarl.LoadProfileWith(c.profile, sets...)
	if err != nil {
		return nil, nil, err
	}
	scn, err := scenario.ParseFile(c.scenario)
	if err != nil {
		return nil, nil, err
	}
	eng, err := snarl.New(p, rate, snarl.Tier1)
	if err != nil {
		return nil, nil, err
	}
	frames := int(math.Round(scn.Duration() * float64(rate)))
	x := make([]float64, 0, frames)
	var starts []int
	var count uint32
	buf := make([]float32, levelBlock)
	for pos := 0; pos < frames; pos += levelBlock {
		rpm, thr := scn.At(float64(pos) / float64(rate))
		s := snarl.Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: snarl.FlagRunning}
		if thr < overrunThrottle {
			s.Flags |= snarl.FlagOverrun
		}
		if noPops {
			s.Flags |= snarl.FlagNoPops
		}
		eng.Set(s)
		b := buf[:min(levelBlock, frames-pos)]
		eng.Render(b)
		for _, v := range b {
			x = append(x, float64(v))
		}
		if r := eng.Pops(); r.Count != count {
			count = r.Count
			starts = append(starts, pos)
		}
	}
	return x, starts, nil
}

// popLevel is the level of one pop over the engine with no pops, in
// decibels, over the span from its start that holds 90 per cent of its
// energy: rmsDB of the plain signals, spanLU of the K-weighted signals,
// and popK and engK the K-weighted over the plain level of the pop and of
// the engine. momLU is the K-weighted level over the momentary window
// from the start. lossDB is the level that the pop limiter takes from the
// pop up to the next pop, and minGain the lowest gain of the pop limiter
// over the samples of the pop above a tenth of its peak.
type popLevel struct {
	rmsDB, spanLU, momLU, popK, engK float64
	lossDB, minGain                  float64
}

// quietGain is the pop gain of the render whose pops the pop limiter does
// not cut, because they are 40 dB down.
const quietGain = 0.01

// kindLevels renders the case with only the kind in use, with no pops and
// with the pops at quietGain, and returns the level of each pop. The
// quiet render less the render with no pops, over quietGain, is the pops
// before the pop limiter: the pop gain sets no random draw.
func kindLevels(c levelCase, kind int) ([]popLevel, error) {
	sets := append([]string(nil), levelSets...)
	for k, n := range popKindNames {
		sets = append(sets, fmt.Sprintf("pops.weight_%s=%d", n, b2i(k == kind)))
	}
	a, starts, err := renderEngine(c, append(sets, "pops.gain=1"), false, levelRate)
	if err != nil {
		return nil, err
	}
	b, _, err := renderEngine(c, sets, true, levelRate)
	if err != nil {
		return nil, err
	}
	q, _, err := renderEngine(c, append(sets, fmt.Sprintf("pops.gain=%g", quietGain)), false, levelRate)
	if err != nil {
		return nil, err
	}
	d, u := make([]float64, len(a)), make([]float64, len(a))
	for i := range a {
		d[i] = a[i] - b[i]
		u[i] = (q[i] - b[i]) / quietGain
	}
	kd, kb := analysis.KWeighted(d, levelRate), analysis.KWeighted(b, levelRate)
	var out []popLevel
	for j, s := range starts {
		// A pop that starts on silence starts at its first sample.
		if s == 0 || d[s-1] == 0 {
			for s < min(len(d)-1, starts[j]+2*levelBlock) && d[s] == 0 {
				s++
			}
		}
		end := min(len(d), s+popSpanS*levelRate)
		if j+1 < len(starts) {
			end = min(end, starts[j+1])
		}
		n := span90(d[s:end], levelRate/100)
		if n == 0 {
			continue
		}
		lv := popLevel{minGain: 1}
		var pd, pb, pkd, pkb float64
		for i := s; i < s+n; i++ {
			pd += d[i] * d[i]
			pb += b[i] * b[i]
			pkd += kd[i] * kd[i]
			pkb += kb[i] * kb[i]
		}
		top := 0.0
		for _, v := range u[s:end] {
			top = max(top, math.Abs(v))
		}
		for i := s; i < end; i++ {
			if math.Abs(u[i]) > 0.1*top {
				lv.minGain = min(lv.minGain, d[i]/u[i])
			}
		}
		lv.rmsDB, lv.spanLU = powDB(pd/pb), powDB(pkd/pkb)
		lv.popK, lv.engK = powDB(pkd/pd), powDB(pkb/pb)
		lv.lossDB = powDB(sumSquares(u[s:end]) / sumSquares(d[s:end]))
		m := min(len(d), s+int(momentaryS*levelRate))
		lv.momLU = powDB(sumSquares(kd[s:m]) / sumSquares(kb[s:m]))
		out = append(out, lv)
	}
	return out, nil
}

// span90 returns the samples from the start of x to the sample at which
// the energy reaches 90 per cent of the total, and at least minN, or 0
// when x has no energy.
func span90(x []float64, minN int) int {
	tot := sumSquares(x)
	if tot == 0 {
		return 0
	}
	var e float64
	for i, v := range x {
		e += v * v
		if e >= 0.9*tot {
			return min(len(x), max(i+1, minN))
		}
	}
	return len(x)
}

// medians returns the medians over the pops of rmsDB, spanLU, momLU, popK,
// engK and lossDB.
func medians(levels []popLevel) [6]float64 {
	var cols [6][]float64
	for _, l := range levels {
		for f, v := range [6]float64{l.rmsDB, l.spanLU, l.momLU, l.popK, l.engK, l.lossDB} {
			cols[f] = append(cols[f], v)
		}
	}
	var m [6]float64
	for f := range cols {
		m[f] = median(cols[f])
	}
	return m
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// popLevels writes to w the levels over the engine of each pop kind on
// each case, as a Markdown table, with the race character.
func popLevels(w io.Writer, args []string) error {
	cs, err := parseCases(args)
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return errors.New("--pop-level needs one or more PROFILE=SCENARIO")
	}
	type result struct {
		levels []popLevel
		err    error
	}
	res := make([][]result, len(cs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, c := range cs {
		res[i] = make([]result, len(popKindNames))
		for k := range popKindNames {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				r := &res[i][k]
				r.levels, r.err = kindLevels(c, k)
			})
		}
	}
	wg.Wait()
	fmt.Fprintln(w, "Levels over the engine with no pops, in dB, the medians over the pops, with the race character. RMS and span are over the span that holds 90 per cent of the energy of the pop; momentary is over the 400 ms from its start. Pop K and engine K are the K-weighted level over the plain level. Loss is the level that the pop limiter takes from the pop. Cut is the pops that lose more than 0.1 dB, and least gain the lowest gain of the pop limiter, in dB.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Engine | Kind | Pops | RMS, dB | Span, LU | Momentary, LU | Pop K, dB | Engine K, dB | Loss, dB | Cut | Most loss, dB | Least gain, dB |")
	fmt.Fprintln(w, "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for i, c := range cs {
		for k, name := range popKindNames {
			r := res[i][k]
			if r.err != nil {
				return fmt.Errorf("%s, %s: %w", c.profile, name, r.err)
			}
			cut, most, least := 0, 0.0, 1.0
			for _, l := range r.levels {
				cut += b2i(l.lossDB > 0.1)
				most, least = max(most, l.lossDB), min(least, l.minGain)
			}
			fmt.Fprintf(w, "| %s | %s | %d |", c.profile, name, len(r.levels))
			for _, v := range medians(r.levels) {
				fmt.Fprintf(w, " %s |", num(v, 1))
			}
			fmt.Fprintf(w, " %d | %s | %s |\n", cut, num(most, 1), num(ampDB(least), 1))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "The mean over the engines of the medians:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Kind | RMS, dB | Span, LU | Momentary, LU | Pop K, dB | Engine K, dB | Loss, dB |")
	fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	for k, name := range popKindNames {
		var mean [6]float64
		for i := range cs {
			for f, v := range medians(res[i][k].levels) {
				mean[f] += v / float64(len(cs))
			}
		}
		fmt.Fprintf(w, "| %s |", name)
		for _, v := range mean {
			fmt.Fprintf(w, " %s |", num(v, 1))
		}
		fmt.Fprintln(w)
	}
	return nil
}
