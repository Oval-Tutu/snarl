package core

import (
	"math"
	"sync"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/dsp"
)

// The calibration of the kind gains plays calDraws pops of each kind and
// source on each channel through the synthesiser, without the leveller,
// with calGapSec of silence after each. calTrimDB raises the gains,
// because the median of the random pops in an engine is about 0.5 dB
// below the mean level of the calibration pops.
const (
	calDraws  = 7
	calTrimDB = 0.5
	calGapSec = 0.3
)

// calKey holds every input of the calibration: the tables, the impulse
// responses and the pop shape settings. The pop settings that only the
// triggers and the gains of the own and requested pops read are left
// out, so that profiles that differ only in them share one calibration.
type calKey struct {
	tables                 *cycles.Tables
	irs                    [MaxChannels]*int16
	n                      [MaxChannels]int
	bodyHz, bodyMS, jumpMS float64
}

// calEntry is one calibration. once runs it, so that concurrent calls of
// New for one profile calibrate once.
type calEntry struct {
	once sync.Once
	gain [MaxChannels][2][numKinds]float32
	err  error
}

// calCacheMax is the most calibrations that the cache holds.
const calCacheMax = 64

var calCache struct {
	sync.Mutex
	m map[calKey]*calEntry
}

// popGains returns the kind gains of the profile, from the cache or from
// a new calibration.
func popGains(s *Spec, res Resources, t *cycles.Tables) ([MaxChannels][2][numKinds]float32, error) {
	k := calKey{tables: t, bodyHz: s.Pops.BodyHz, bodyMS: s.Pops.BodyMS, jumpMS: s.Pops.JumpMS}
	for i, ir := range res.ImpulseResponses {
		if len(ir) > 0 {
			k.irs[i], k.n[i] = &ir[0], len(ir)
		}
	}
	calCache.Lock()
	e := calCache.m[k]
	if e == nil {
		if calCache.m == nil {
			calCache.m = make(map[calKey]*calEntry)
		}
		if len(calCache.m) >= calCacheMax {
			for old := range calCache.m {
				delete(calCache.m, old)
				break
			}
		}
		e = &calEntry{}
		calCache.m[k] = e
	}
	calCache.Unlock()
	e.once.Do(func() { e.gain, e.err = calibrate(s, res, t) })
	return e.gain, e.err
}

// calibrate measures the level of each pop kind on each channel: the RMS
// level of the synthesiser output before the leveller over the span from
// the start that holds 90 per cent of the energy, for a pop at unit gain.
// The gain of a kind is set from the mean level in decibels, so that a pop
// of unit scale on an engine whose output before the leveller has unit RMS
// level is at its overDB target. The body frequency, body decay and
// sub-event count of the pops cover their spreads in even steps, and the
// sub-events follow a low-discrepancy sequence. It allocates.
func calibrate(s *Spec, res Resources, t *cycles.Tables) ([MaxChannels][2][numKinds]float32, error) {
	var gain [MaxChannels][2][numKinds]float32
	for i := range gain {
		for r := range gain[i] {
			for k := range gain[i][r] {
				gain[i][r][k] = 1
			}
		}
	}
	nc := len(t.Channels)
	c := newPops(s, t.Rate, block/float64(synthRate))
	c.noise = dsp.NewNoise(0xca1)
	in := make([]float64, nc)
	out := make([]float32, block)
	gap := int(math.Ceil(calGapSec * synthRate / block))
	for ch := range nc {
		rt, err := newRuntime(t, res.ImpulseResponses, 1, synthRate, true)
		if err != nil {
			return gain, err
		}
		// run renders one convolution block of the pops on channel ch.
		run := func() {
			for rt.InputSize() < block {
				c.tick()
				clear(in)
				if c.active > 0 {
					in[ch] = float64(c.jump[ch])
				}
				rt.WriteInput(in)
			}
			rt.Render(out)
		}
		var buf []float32
		tail := int(math.Ceil(float64(len(res.ImpulseResponses[ch])+2*block)/block)) + 1
		for slot := range 2 * numKinds {
			kind, src := slot%numKinds, srcOwn
			if slot >= numKinds {
				src = srcRequest
			}
			var lg float64
			for d := range calDraws {
				c.target, c.stratum, c.drawN = ch, d+1, 0
				c.subU = float32(d) / calDraws
				e := c.fire(nc, kind, src, 1, 1, 1)
				buf = buf[:0]
				for left := tail; left > 0; {
					run()
					buf = append(buf, out...)
					if e.left == 0 {
						left--
					}
				}
				c.stratum = 0
				for range gap {
					run()
				}
				lg += math.Log10(math.Max(spanRMS(buf, synthRate/100), 1e-12))
			}
			gain[ch][slot/numKinds][kind] = float32(math.Pow(10, (overDB[kind]+calTrimDB)/20-lg/calDraws))
		}
	}
	return gain, nil
}

// spanRMS returns the RMS level of x from its start to the sample at which
// the energy reaches 90 per cent of the total, and over at least minN
// samples.
func spanRMS(x []float32, minN int) float64 {
	var tot float64
	for _, v := range x {
		tot += float64(v) * float64(v)
	}
	var e float64
	for i, v := range x {
		e += float64(v) * float64(v)
		if e >= 0.9*tot {
			n := min(len(x), max(i+1, minN))
			var w float64
			for _, u := range x[:n] {
				w += float64(u) * float64(u)
			}
			return math.Sqrt(w / float64(n))
		}
	}
	return 0
}
