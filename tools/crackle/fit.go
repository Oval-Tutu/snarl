package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// modelRate is the sample rate of the model trains of the fit and the
	// check, the rate of the clips.
	modelRate = 44100
	// fitTrains is the number of model trains of each step of the fit,
	// with the seeds 1 to fitTrains in every step. The check uses other
	// seeds (checkSeed).
	fitTrains = 96
	// trimSteps is the number of steps of the band powers of the banks
	// before the search, and fitGain the part of the error that
	// each step corrects.
	trimSteps = 15
	fitGain   = 0.4
	// searchEvals and weightEvals are the most evaluations of the two
	// searches, and searchClip
	// the largest square of the distance of one measure in the loss.
	searchEvals = 600
	weightEvals = 900
	searchClip  = 16
	// checkSeed is the seed of the first train of the check.
	checkSeed = 5001
	// bankGroup is the number of trains of one bank in the fit and the
	// check.
	bankGroup = 16
	// fadeInMS is the fade at the start of a train. The clips start cut,
	// with no fade, so it is a snarl default against a click.
	fadeInMS = 1
	// actMS and actDB are the start of the slow ring of the shock
	// activity for the search.
	actMS = 30
	actDB = -20
	// The frames of the band energy over time that the fit matches, in
	// milliseconds from the frame of the strongest shock: the background
	// before the shock, the shock, and the decay after it.
	bgFrame1, bgFrame2 = -10, -5
	shockFrame         = 5
	decayFrame         = 10
	ringFrame          = 20
	lateFrame          = 40
	// trimDB is the largest correction of the band powers by the fit
	// steps.
	trimDB = 20
	// offDB is the gain of a band with no energy in a bank.
	offDB = -200
	// The search for the corner of the low pass at the top of the banks.
	topLoHz, topHiHz, topStepHz = 8000.0, 19000.0, 500.0
	// rlSteps is the number of Richardson-Lucy steps of the bank fit. The
	// band powers of the bank filters come from rlLen samples of their
	// carriers, less the first rlSkip, and the body from rlLen samples of
	// its impulse response.
	rlSteps = 500
	rlLen   = modelRate
	rlSkip  = modelRate / 10
)

// stats are the measures of a set of clips that the fit matches.
type stats struct {
	// med holds the median over the clips of each measure of allRows, by
	// column.
	med map[string]float64
	// gapQ and levelQ are the quantile tables of the gaps between all
	// shocks in milliseconds and of the levels of all shocks, pooled.
	gapQ, levelQ [crackle.Quantiles]float64
	// gaps and shocks are the counts of the gaps and the shocks.
	gaps, shocks int
	// place is the median level of the shocks in each tenth of the active
	// span, pooled, against the pooled median level.
	place [10]float64
	// strongCrest is the median crest factor of the strong shocks, pooled,
	// and strongShare the part of all shocks that are strong. strongFall is
	// the median fall of the strong shocks, pooled, and serial the
	// correlation of the levels of consecutive shocks, pooled.
	strongCrest, strongShare, strongFall, serial float64
	// bg, onset, shock and decay are the medians over the clips of the band
	// energy over time (frameDB) in the background frames, in the frames of
	// the onset and the shock and in the frame of the decay, for each band and above
	// 2 kHz. bg is the mean in dB of the two background frames.
	bg, onset, shock, decay, ring, late [crackle.Bands + 1]float64
}

func column(name string) measureRow {
	for _, r := range allRows {
		if r.column == name {
			return r
		}
	}
	panic("crackle: no measure " + name)
}

// over returns the values of the measure in the column over the clips.
func over(clips []*clip, name string) []float64 {
	r := column(name)
	var x []float64
	for _, c := range clips {
		x = append(x, r.get(c))
	}
	return x
}

func median(x []float64) float64 { return quantile(finite(x), 0.5) }

func quantiles(x []float64) (q [crackle.Quantiles]float64) {
	v := finite(x)
	for i := range q {
		q[i] = quantile(v, (float64(i)+0.5)/float64(crackle.Quantiles))
	}
	return q
}

// frameMedians returns the median over the clips of frameDB at off.
func frameMedians(clips []*clip, off int) (out [crackle.Bands + 1]float64) {
	cols := make([][]float64, len(out))
	for _, c := range clips {
		for i, v := range frameDB(c, off) {
			cols[i] = append(cols[i], v)
		}
	}
	for i := range out {
		out[i] = median(cols[i])
	}
	return out
}

func statsOf(clips []*clip) stats {
	s := stats{med: map[string]float64{}}
	for _, r := range allRows {
		s.med[r.column] = median(over(clips, r.column))
	}
	var g, l, crest, fall, prev, next []float64
	var place [10][]float64
	for _, c := range clips {
		for _, v := range gaps(c, false) {
			g = append(g, v*1000)
		}
		for k, sh := range c.Shocks {
			l = append(l, sh.RelDB)
			d := min(9, int(sh.T/c.Length*10))
			place[d] = append(place[d], sh.RelDB)
			if sh.Strong {
				crest = append(crest, sh.CrestDB)
				fall = append(fall, sh.FallMS)
			}
			if k > 0 {
				prev, next = append(prev, c.Shocks[k-1].RelDB), append(next, sh.RelDB)
			}
		}
	}
	s.strongFall, s.serial = median(fall), pearson(prev, next)
	s.gapQ, s.levelQ = quantiles(g), quantiles(l)
	s.gaps, s.shocks = len(finite(g)), len(l)
	all := median(l)
	for d := range place {
		s.place[d] = median(place[d]) - all
	}
	s.strongCrest = median(crest)
	s.strongShare = float64(len(crest)) / float64(len(l))
	b1, b2 := frameMedians(clips, bgFrame1), frameMedians(clips, bgFrame2)
	for i := range s.bg {
		s.bg[i] = (b1[i] + b2[i]) / 2
	}
	s.onset, s.shock, s.decay = frameMedians(clips, 0), frameMedians(clips, shockFrame), frameMedians(clips, decayFrame)
	s.ring, s.late = frameMedians(clips, ringFrame), frameMedians(clips, lateFrame)
	return s
}

// renderModel renders a train for each seed, with a bank for each group of
// bankGroup seeds, whose seed is the first seed of the group, so that the
// fit does not follow the grains of one bank. It scales each train to the
// sample peak peakDB in dBFS, as the pack scales its clips, and measures
// it. It returns the measures, the sample peak of each train before the
// scaling, and the scaled trains.
func renderModel(p *crackle.Params, seeds []uint32, peakDB float64) ([]*clip, []float64, [][]float32, error) {
	clips := make([]*clip, len(seeds))
	raw := make([]float64, len(seeds))
	trains := make([][]float32, len(seeds))
	errs := make([]error, len(seeds))
	banks := make([]*crackle.Bank, (len(seeds)+bankGroup-1)/bankGroup)
	for i := range banks {
		banks[i] = crackle.NewBank(p, modelRate, seeds[i*bankGroup])
	}
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Go(func() {
			y := crackle.Render(banks[i/bankGroup], seed, crackle.Options{})
			var pk float64
			for _, v := range y {
				pk = max(pk, math.Abs(float64(v)))
			}
			raw[i] = ampDB(pk)
			g := float32(dbAmp(peakDB) / pk)
			x := make([]float64, len(y))
			for j := range y {
				y[j] *= g
				x[j] = float64(y[j])
			}
			trains[i] = y
			clips[i], errs[i] = measure(fmt.Sprintf("model_%d", seed), &wav.Audio{SampleRate: modelRate, Channels: [][]float64{x}}, dbAmp(defaultFloorDB))
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, nil, nil, fmt.Errorf("model train of seed %d: %w", seeds[i], err)
		}
	}
	return clips, raw, trains, nil
}

func seedRange(first, n int) []uint32 {
	s := make([]uint32, n)
	for i := range s {
		s[i] = uint32(first + i)
	}
	return s
}

// step returns the correction of a value by the error e, limited to lim.
func step(e, lim float64) float64 {
	if math.IsNaN(e) || math.IsInf(e, 0) {
		return 0
	}
	return max(-lim, min(lim, fitGain*e))
}

// bankMatrix returns the power of the carrier of each band of the banks,
// through its filter and the low pass at the top, in each band of the
// measures: h[j][b] for band j of the bank and measure band b.
func bankMatrix(topHz float64) [][]float64 {
	h := make([][]float64, crackle.Bands)
	for j := range h {
		x := crackle.BankSignal(j, topHz, modelRate, rlLen)[rlSkip:]
		h[j] = make([]float64, len(bands))
		for b, e := range bands {
			h[j][b] = meanSquare(analysis.BandPass(x, modelRate, e[0], e[1])[rlSkip:])
		}
	}
	return h
}

// fitBank returns the gains in dB of the bank whose band powers come
// nearest to the target band powers, by Richardson-Lucy steps on the
// squared gains. The band powers of the bank add, because each band has
// its own carrier.
func fitBank(h [][]float64, target [crackle.Bands]float64) (g [crackle.Bands]float64) {
	t := target[:]
	w := make([]float64, len(h))
	for j := range w {
		if h[j][j] > 0 {
			w[j] = t[j] / h[j][j]
		}
	}
	pw := make([]float64, len(t))
	for range rlSteps {
		for b := range pw {
			pw[b] = 0
			for j := range w {
				pw[b] += w[j] * h[j][b]
			}
		}
		for j := range w {
			var num, den float64
			for b := range t {
				if pw[b] > 0 {
					num += h[j][b] * t[b] / pw[b]
				}
				den += h[j][b]
			}
			if den > 0 {
				w[j] *= num / den
			}
		}
	}
	for j := range g {
		g[j] = max(offDB, powDB(w[j]))
	}
	return g
}

// fitState holds the parameters and the band powers that the banks and
// the body must give, from the band energy over time of the clips, both
// against the loudest frame of a clip.
type fitState struct {
	p *crackle.Params
	// bed is the power of each band of the background in each sample. shock
	// is the energy of each band of the strongest shock above the
	// background in the frame of its onset and the frame after it, and ring
	// the same in the frames from 10 to 40 ms after it. body is the
	// background power from 31.25 to 62.5 Hz, which the body alone gives.
	bed, shock, ring [crackle.Bands]float64
	// The trims are the corrections of bed, shock and ring in dB by the fit
	// steps: the background of the clips holds weak shocks, which the model
	// adds to its bed.
	bedTrim, shockTrim, ringTrim [crackle.Bands]float64
	// The levels are the corrections of all bands of bed, shock and ring
	// in dB by the search.
	bedLevel, shockLevel, ringLevel float64
	// bodyLevel is the correction of the body power in dB by the search,
	// placeScale the scale of the measured place offsets placeBase, and
	// hTop the corner of the low pass at the top for which h holds.
	bodyLevel, placeScale, hTop float64
	placeBase                   [10]float64
	body                        float64
	h                           [][]float64
}

// initial returns the parameters that the measures of the clips give
// before the fit.
func initial(clips []*clip, t stats) *fitState {
	p := &crackle.Params{
		LengthOffsetMS: t.med["tail_ms"] + tailGapS*1000,
		GapMS:          t.gapQ, LevelDB: t.levelQ, GapScale: 1, LevelScale: 1, PlaceDB: t.place,
		AttackMS: t.med["strong_rise_ms"] / 0.8, DecayMS: t.strongFall, LevelCorr: t.serial,
		RingMS: -2 * (ringFrame - shockFrame) / (t.ring[crackle.Bands] - t.shock[crackle.Bands]) * 10 / math.Ln10,
		BodyHz: t.med["body_hz"], BodyQ: 2,
		FadeDelayMS: t.med["tail_ms"], FadeDBPerMS: -t.med["end_fall_db"] / (endFallS * 1000),
		FadeInMS: fadeInMS, ActMS: actMS, ActDB: actDB,
		TailHz: t.med["tail_cycle_hz"], TailDecayMS: t.med["tail_decay20_ms"],
	}
	lengths := finite(over(clips, "active_ms"))
	for i := range p.LengthMS {
		p.LengthMS[i] = quantile(lengths, float64(i)/4)
	}
	n := finite(over(clips, "shocks"))
	p.ShocksMin, p.ShocksMax = int(n[0]), int(n[len(n)-1])
	s := &fitState{p: p, body: math.Pow(10, t.bg[1]/10), placeScale: 1, placeBase: t.place}
	frame := frameS * modelRate
	for b := lowBands; b < crackle.Bands; b++ {
		bg := math.Pow(10, t.bg[b]/10)
		s.bed[b] = bg
		for _, f := range [][crackle.Bands + 1]float64{t.onset, t.shock} {
			s.shock[b] += max(0, math.Pow(10, f[b]/10)-bg) * frame
		}
		// The frame 40 ms after the shock stands for the frames from 25 to
		// 40 ms.
		for i, f := range [][crackle.Bands + 1]float64{t.decay, t.ring, t.late} {
			s.ring[b] += max(0, math.Pow(10, f[b]/10)-bg) * frame * [3]float64{1, 3, 3}[i]
		}
	}
	// The corner of the low pass at the top is the one at which the banks
	// come nearest to their band powers.
	best := math.Inf(1)
	for hz := topLoHz; hz <= topHiHz; hz += topStepHz {
		s.h = bankMatrix(hz)
		if e := s.bankError(); e < best {
			best, p.TopHz = e, hz
		}
	}
	s.h, s.hTop = bankMatrix(p.TopHz), p.TopHz
	s.solve()
	// The tail starts at the peak of a sine with the RMS level of the body.
	h := sumSquares(crackle.BodyResponse(p.BodyHz, p.BodyQ, modelRate, rlLen))
	p.TailGainDB = p.BodyGainDB + powDB(crackle.NoisePower*h*2)
	return s
}

// targets returns the band powers that the banks of the bed, the shock
// and the ring must give at the gain 1 and for a shock at 0 dB.
func (s *fitState) targets() (bed, shock, ring [crackle.Bands]float64) {
	ec, er := s.p.EnvelopeEnergy(modelRate)
	for b := range bed {
		bed[b] = s.bed[b] * math.Pow(10, (s.bedTrim[b]+s.bedLevel)/10)
		shock[b] = s.shock[b] * math.Pow(10, (s.shockTrim[b]+s.shockLevel)/10) / ec
		ring[b] = s.ring[b] * math.Pow(10, (s.ringTrim[b]+s.ringLevel)/10) / er
	}
	return bed, shock, ring
}

// bankError returns the squared error in dB of the band powers of the
// banks fitted with the matrix s.h against their targets.
func (s *fitState) bankError() float64 {
	var sum float64
	bed, shock, ring := s.targets()
	for _, t := range [][crackle.Bands]float64{bed, shock, ring} {
		g := fitBank(s.h, noTop(t))
		for b := lowBands; b < crackle.Bands; b++ {
			var p float64
			for j := range g {
				p += math.Pow(10, g[j]/10) * s.h[j][b]
			}
			if t[b] > 0 {
				d := powDB(p / t[b])
				sum += d * d
			}
		}
	}
	return sum
}

// noTop returns the band powers with none in the top band. The top band
// of the clips is the leak of the band below it through the low pass at
// the top, so the banks have no filter there.
func noTop(t [crackle.Bands]float64) [crackle.Bands]float64 {
	t[crackle.Bands-1] = 0
	return t
}

// solve sets the gains of the banks and of the body from the band powers.
func (s *fitState) solve() {
	if s.hTop != s.p.TopHz {
		s.h, s.hTop = bankMatrix(s.p.TopHz), s.p.TopHz
	}
	bed, shock, ring := s.targets()
	s.p.BedDB, s.p.ShockDB, s.p.RingDB = fitBank(s.h, noTop(bed)), fitBank(s.h, noTop(shock)), fitBank(s.h, noTop(ring))
	e := bands[1]
	h := sumSquares(analysis.BandPass(crackle.BodyResponse(s.p.BodyHz, s.p.BodyQ, modelRate, rlLen), modelRate, e[0], e[1]))
	s.p.BodyGainDB = powDB(s.body * math.Pow(10, s.bodyLevel/10) / (crackle.NoisePower * h))
	for d, v := range s.placeBase {
		s.p.PlaceDB[d] = s.placeScale * v
	}
}

// trimStep corrects the band powers of the banks by the errors of the band
// energy over time of the model measures m against the clip measures t.
// The band energy is against the loudest frame of a train. The body stays
// at its band power, so the error of the body band moves the bed and the
// shocks against it.
func (s *fitState) trimStep(t, m stats) {
	body := t.bg[1] - m.bg[1]
	for b := lowBands; b < crackle.Bands-1; b++ {
		s.bedTrim[b] = min(trimDB, max(-trimDB, s.bedTrim[b]+step(t.bg[b]-m.bg[b]-body, 3)))
		s.shockTrim[b] = min(trimDB, max(-trimDB, s.shockTrim[b]+step(t.shock[b]-m.shock[b]-body, 3)))
		s.ringTrim[b] = min(trimDB, max(-trimDB, s.ringTrim[b]+step(t.ring[b]-m.ring[b]-body, 3)))
	}
	s.solve()
}

// knob is one parameter of the search, with its range and its first step.
// A knob with log steps by a factor of exp(step).
type knob struct {
	name         string
	v            func(s *fitState) *float64
	lo, hi, step float64
	log          bool
}

// knobs are the parameters of the search. The tables of the gaps and the
// levels stay the measured tables: the detector misses and merges shocks,
// so the search scales the gaps and the spread of the levels.
var knobs = []knob{
	{"gap_scale", func(s *fitState) *float64 { return &s.p.GapScale }, 0.25, 4, math.Ln2, true},
	{"level_scale", func(s *fitState) *float64 { return &s.p.LevelScale }, 0.25, 4, math.Ln2, true},
	{"level_corr", func(s *fitState) *float64 { return &s.p.LevelCorr }, 0, 0.9, 0.2, false},
	{"attack_ms", func(s *fitState) *float64 { return &s.p.AttackMS }, 0.1, 10, math.Ln2, true},
	{"decay_ms", func(s *fitState) *float64 { return &s.p.DecayMS }, 0.02, 5, math.Ln2, true},
	{"ring_ms", func(s *fitState) *float64 { return &s.p.RingMS }, 0.5, 60, math.Ln2, true},
	{"ring level", func(s *fitState) *float64 { return &s.ringLevel }, -20, 20, 6, false},
	{"bed level", func(s *fitState) *float64 { return &s.bedLevel }, -20, 20, 6, false},
	{"shock level", func(s *fitState) *float64 { return &s.shockLevel }, -20, 20, 6, false},
	{"body level", func(s *fitState) *float64 { return &s.bodyLevel }, -20, 20, 6, false},
	{"place scale", func(s *fitState) *float64 { return &s.placeScale }, 0, 1, 0.5, false},
	{"place scale wide", func(s *fitState) *float64 { return &s.placeScale }, 0, 2, 0.5, false},
	{"place_weight", func(s *fitState) *float64 { return &s.p.PlaceWeight }, 0, 1, 0.5, false},
	{"top_hz", func(s *fitState) *float64 { return &s.p.TopHz }, 8000, 19000, math.Ln2 / 4, true},
	{"act_ms", func(s *fitState) *float64 { return &s.p.ActMS }, 5, 300, math.Ln2, true},
	{"act_db", func(s *fitState) *float64 { return &s.p.ActDB }, -60, 10, 6, false},
	{"body_hz", func(s *fitState) *float64 { return &s.p.BodyHz }, 30, 80, math.Ln2 / 4, true},
	{"body_q", func(s *fitState) *float64 { return &s.p.BodyQ }, 0.3, 10, math.Ln2, true},
	{"tail_gain_db", func(s *fitState) *float64 { return &s.p.TailGainDB }, -40, 20, 6, false},
	{"fade_db_per_ms", func(s *fitState) *float64 { return &s.p.FadeDBPerMS }, 0.2, 5, math.Ln2 / 2, true},
	{"fade_delay_ms", func(s *fitState) *float64 { return &s.p.FadeDelayMS }, 0, 200, 30, false},
	{"length_offset_ms", func(s *fitState) *float64 { return &s.p.LengthOffsetMS }, 0, 200, 30, false},
}

// skipColumns are the clip measures that the search leaves out: the edges
// of the files, which the edit of the pack sets, the channels, which the
// mono model does not have, and the sample peak, which the scaling sets.
var skipColumns = map[string]bool{
	"start_ms": true, "head_dbfs": true, "tail_dbfs": true, "bursts": true, "quiet_ms": true,
	"late_ms": true, "late_dbfs": true, "peak_dbfs": true,
	"corr": true, "lag_corr": true, "delay_ms": true, "level_diff_db": true, "side_db": true,
}

// loss returns the error of the model trains against the clips: for each
// section of the check, the mean over its measures of the square of the
// distance of the model median from the clip median over the clip
// interquartile range, limited to searchClip.
func loss(clips, model []*clip) float64 {
	sum := map[string]float64{}
	n := map[string]int{}
	for _, r := range compareRows(clips, model) {
		if skipColumns[r.column] {
			continue
		}
		c, m := finite(r.clips), finite(r.model)
		if len(c) == 0 || len(m) == 0 {
			continue
		}
		med := quantile(c, 0.5)
		iqr := max(quantile(c, 0.75)-quantile(c, 0.25), 0.05*math.Abs(med), 1e-3)
		z := (quantile(m, 0.5) - med) / iqr
		sum[r.section] += min(z*z, searchClip)
		n[r.section]++
	}
	var total float64
	for k, v := range sum {
		total += v / float64(n[k])
	}
	return total
}

// search moves each knob in turn by its step up and down, and keeps a move
// that lowers the loss. A knob that no move improves halves its step. It
// leaves out the knobs in skip, and stops after budget evaluations.
func (s *fitState) search(clips []*clip, seeds []uint32, peak float64, budget int, log io.Writer, skip ...string) error {
	eval := func() (float64, error) {
		s.solve()
		mc, _, _, err := renderModel(s.p, seeds, peak)
		if err != nil {
			return 0, err
		}
		return loss(clips, mc), nil
	}
	best, err := eval()
	if err != nil {
		return err
	}
	steps := make([]float64, len(knobs))
	for i, k := range knobs {
		steps[i] = k.step
	}
	evals := 1
	for evals < budget {
		moved := false
		for i, k := range knobs {
			if slices.Contains(skip, k.name) {
				continue
			}
			x := k.v(s)
			old := *x
			improved := false
			for _, dir := range []float64{1, -1} {
				v := old + dir*steps[i]
				if k.log {
					v = old * math.Exp(dir*steps[i])
				}
				*x = min(k.hi, max(k.lo, v))
				if *x == old {
					continue
				}
				l, err := eval()
				if err != nil {
					return err
				}
				evals++
				if l < best {
					best, improved = l, true
					break
				}
				*x = old
			}
			if improved {
				moved = true
			} else {
				steps[i] /= 2
			}
		}
		fmt.Fprintf(log, "search: %d evaluations, loss %.3f\n", evals, best)
		if !moved && steps[0] < knobs[0].step/64 {
			break
		}
	}
	s.solve()
	return nil
}

// fitResult is the outcome of the fit.
type fitResult struct {
	p        *crackle.Params
	clip     stats
	model    stats
	rawPeaks []float64
	loss     float64
}

// fit fits the parameters of the model to the clips: steps of the band
// powers of the banks, then two searches of the knobs.
func fit(clips []*clip, log io.Writer) (*fitResult, error) {
	t := statsOf(clips)
	s := initial(clips, t)
	p := s.p
	peak := t.med["peak_dbfs"]
	seeds := seedRange(1, fitTrains)
	for range trimSteps {
		mc, _, _, err := renderModel(p, seeds, peak)
		if err != nil {
			return nil, err
		}
		s.trimStep(t, statsOf(mc))
	}
	// The first search puts the place offsets on the whole level, with a
	// scale up to 1. The second starts with them on the median and not the
	// top, with a scale up to 2, from the values of the first, and frees
	// their weight, so that the loss on the fit trains selects it.
	p.PlaceWeight = 0
	if err := s.search(clips, seeds, peak, searchEvals, log, "place scale wide", "place_weight"); err != nil {
		return nil, err
	}
	p.PlaceWeight = 1
	if err := s.search(clips, seeds, peak, weightEvals, log, "place scale"); err != nil {
		return nil, err
	}
	mc, raw, _, err := renderModel(p, seeds, peak)
	if err != nil {
		return nil, err
	}
	p.OutDB = peak - median(raw)
	return &fitResult{p: p, clip: t, model: statsOf(mc), rawPeaks: raw, loss: loss(clips, mc)}, nil
}

// source is one entry of the sources file.
type source struct{ key, kind, from string }

func (f *fitResult) sources() []source {
	t, m := f.clip, f.model
	searched := func(start string) string {
		return fmt.Sprintf("fitted by the two searches of tools/crackle --fit (just crackle-fit): the compass searches of %d knobs that lower the loss over the measures of the check, the distance of the model median from the clip median over the clip interquartile range, on %d model trains with the seeds 1 to %d, to %.3f; the start is %s", len(knobs), fitTrains, fitTrains, f.loss, start)
	}
	bank := func(what string) string {
		return fmt.Sprintf("fitted by tools/crackle --fit: the gains by %d Richardson-Lucy steps that give the band powers of %s, against the loudest frame of a clip, from the medians over the 30 clips of the band energy over time around the strongest shock (tools/crackle), less the background; %d steps of the fit correct the band powers by the errors of the band energy of the model trains, and the search moves the level of all bands", rlSteps, what, trimSteps)
	}
	return []source{
		{"length_ms", "measured", "the minimum, quartiles and maximum of the active length of the 30 clips (tools/crackle), the range of the length control"},
		{"length_offset_ms", "fitted", searched(fmt.Sprintf("the median tail after the last shock of the clips and 10 ms; the median active length is %.0f ms for the clips and %.0f ms for the model", t.med["active_ms"], m.med["active_ms"]))},
		{"shocks_range", "measured", "the least and the most shocks of a clip over the 30 clips (tools/crackle), the range of the length control"},
		{"gap_ms", "measured", fmt.Sprintf("the quantiles at the probabilities (i + 0.5) / 21 of the %d gaps between all shocks of the 30 clips, pooled (tools/crackle)", t.gaps)},
		{"gap_scale", "fitted", searched(fmt.Sprintf("1; the median shock rate is %.0f per second for the clips and %.0f for the model", t.med["shock_rate"], m.med["shock_rate"]))},
		{"level_db", "measured", fmt.Sprintf("the quantiles at the probabilities (i + 0.5) / 21 of the level of the %d shocks of the 30 clips against the strongest shock of their clip, pooled (tools/crackle)", t.shocks)},
		{"level_scale", "fitted", searched(fmt.Sprintf("1; %.2f of the shocks are strong for the clips and %.2f for the model", t.strongShare, m.strongShare))},
		{"level_corr", "fitted", searched(fmt.Sprintf("the correlation of the levels of consecutive shocks of the clips, pooled, %.2f; it is %.2f for the model", t.serial, m.serial))},
		{"place_db", "fitted", searched("the median level of the shocks in each tenth of the active span of the clips, pooled, against the pooled median level, with a scale that the search moves from 1")},
		{"place_weight", "fitted", searched("0 in the first search, which holds it, and 1 in the second, which frees it")},
		{"attack_ms", "fitted", searched(fmt.Sprintf("the median rise above 1 kHz of the strong shocks of the clips over 0.8; the median rise is %.2f ms for the clips and %.2f ms for the model", t.med["strong_rise_ms"], m.med["strong_rise_ms"]))},
		{"decay_ms", "fitted", searched(fmt.Sprintf("the median fall of the strong shocks of the clips to -10 dB, %.2f ms; it is %.2f ms for the model", t.strongFall, m.strongFall))},
		{"ring_ms", "fitted", searched("the fall above 2 kHz from the frame 5 ms after the strongest shock to the frame 20 ms after it, median over the clips, as an exponential decay")},
		{"shock_db", "fitted", bank("the frame of the onset of the strongest shock and the frame after it")},
		{"ring_db", "fitted", bank("the frames 10, 20 and 40 ms after the strongest shock")},
		{"bed_db", "fitted", bank("the frames 10 and 5 ms before the strongest shock; the bands below 125 Hz are the body alone")},
		{"top_hz", "fitted", searched("the corner from 8 to 19 kHz at which the banks come nearest to their band powers")},
		{"body_hz", "fitted", searched(fmt.Sprintf("the median body peak of the clips; it is %.1f Hz for the clips and %.1f Hz for the model", t.med["body_hz"], m.med["body_hz"]))},
		{"body_q", "fitted", searched(fmt.Sprintf("2; the median spread of the body cycles is %.2f for the clips and %.2f for the model", t.med["cycle_spread"], m.med["cycle_spread"]))},
		{"body_gain_db", "fitted", "fitted by tools/crackle --fit: the gain of the body filter for white noise that gives the median over the 30 clips of the band energy from 31.25 to 62.5 Hz in the frames 10 and 5 ms before the strongest shock, against the loudest frame, with a level that the search moves from 0 dB"},
		{"fade_delay_ms", "fitted", searched(fmt.Sprintf("the median tail after the last shock of the clips; it is %.0f ms for the clips and %.0f ms for the model", t.med["tail_ms"], m.med["tail_ms"]))},
		{"fade_db_per_ms", "fitted", searched(fmt.Sprintf("the median fall over the last 20 ms of the active span of the clips over 20 ms; the fall is %.1f dB for the clips and %.1f dB for the model", t.med["end_fall_db"], m.med["end_fall_db"]))},
		{"fade_in_ms", "snarl", "a snarl default against a click at the start: the clips start cut, with sound at the first sample, so they cannot give a fade-in"},
		{"act_ms", "fitted", searched("30 ms")},
		{"act_db", "fitted", searched("-20 dB")},
		{"tail_hz", "measured", "the median over the clips of the tail frequency from the median half cycle after the last shock (tools/crackle), n=13"},
		{"tail_decay_ms", "measured", "the median over the clips of the tail decay to -20 dB after the last shock (tools/crackle), n=13"},
		{"tail_gain_db", "fitted", searched(fmt.Sprintf("the peak of a sine with the RMS level of the body; the median tail peak is %.0f Hz for the clips and %.0f Hz for the model", t.med["tail_hz"], m.med["tail_hz"]))},
		{"out_db", "fitted", fmt.Sprintf("the gain that brings the median sample peak of the %d fit trains, %.2f dBFS, to the median sample peak of the clips, %.2f dBFS", fitTrains, median(f.rawPeaks), t.med["peak_dbfs"])},
	}
}

// paramsHeader is the header of the parameter file, and sourcesHeader that
// of the sources file.
const (
	paramsHeader  = "Class-level parameters of the crackle model (internal/crackle), written by\njust crackle-fit (tools/crackle --fit, then --fit-tail) from the 30 short\nexterior crackle clips. Do not edit by hand. crackle.sources.toml gives the\nsource of each key, and docs/research/crackle-model.md and\ndocs/research/pop-tail.md the model and the check."
	sourcesHeader = "# Sources of the keys of crackle.toml, written by just crackle-fit.\n# kind: measured (a statistic of the 30 clips by tools/crackle), fitted (a\n# deterministic fit by tools/crackle --fit of model trains to a statistic\n# of the clips), derived (a fit of model trains of a form that no clip\n# holds to a statistic of the clips) or snarl (a snarl default where the\n# clips cannot measure the value).\n"
)

// writeFit writes the parameter file and the sources file beside it. The
// keys of the tail are 0, the tail of the fit trains, until --fit-tail
// sets them.
func writeFit(path string, f *fitResult) error {
	if err := os.WriteFile(path, f.p.Marshal(paramsHeader), 0o600); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString(sourcesHeader)
	for _, s := range f.sources() {
		fmt.Fprintf(&b, "\n[[source]]\nkey = %q\nkind = %q\nfrom = %q\n", s.key, s.kind, s.from)
	}
	return os.WriteFile(sourcesPath(path), []byte(b.String()), 0o600)
}
