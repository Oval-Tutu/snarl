package core

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/crackle"
)

// The calibration of the kind gains renders calDraws voices of each kind
// and source with no engine. calTrimDB raises the gains, because the
// median of the random pops in an engine, over the kinds and the
// profiles, is about 0.1 dB below the mean level of the calibration
// voices (TestPopLevelOverEngine). bankSeed sets the carriers and the
// noise of the shared bank.
const (
	calDraws  = 32
	calTrimDB = 0.1
	bankSeed  = 1
)

// engineKDB is the K-weighted level over the plain RMS level of the engine
// output, in dB: the mean over the five embedded profiles on the lift-offs
// of tools/crackle, -0.4 to +0.7 dB on each (docs/research/pop-level.md).
// The engine level of the pops is a plain RMS level, so the calibration
// adds this offline correction to put the pops over the loudness of the
// engine with no filter in the runtime.
const engineKDB = -0.1

// popModel is the crackle model of the pops at the synthesiser rate,
// which every engine shares and only reads: the parameters, the bank of
// the voices and the kind gains.
type popModel struct {
	params *crackle.Params
	bank   *crackle.Bank
	// gain is the gain of each kind that puts a voice of unit scale on an
	// engine of unit RMS level before the leveller at its loudness target,
	// overLU plus popOffsetLU, for an own pop or a bang and for a
	// requested pop.
	gain [2][numKinds]float32
}

// sharedModel returns the pop model of the process, or the error of the
// parameters of the crackle model. The first call makes the bank and
// calibrates the kind gains.
var sharedModel = sync.OnceValues(func() (*popModel, error) { return newPopModel(crackle.Fitted()) })

// shortModel is the pop model of the crackle model with the short tail
// (crackle.Params.ShortTail), which New uses after UseShortPopTail.
var shortModel = sync.OnceValues(func() (*popModel, error) {
	p, err := crackle.Fitted()
	if err != nil {
		return nil, err
	}
	return newPopModel(p.ShortTail(), nil)
})

var shortPopTail atomic.Bool

// UseShortPopTail makes the engines that New returns after it play the
// pops with the short tail of the crackle model, with no hold and no
// trailing shocks, when on is true. It is for the measures and the
// listening files of tools/crackle, which compare the two tails.
func UseShortPopTail(on bool) { shortPopTail.Store(on) }

// newPopModel returns the pop model of the parameters p, or err when it is
// not nil.
func newPopModel(p *crackle.Params, err error) (*popModel, error) {
	if err != nil {
		return nil, err
	}
	m := &popModel{params: p, bank: crackle.NewBank(p, synthRate, bankSeed)}
	m.calibrate()
	return m, nil
}

// calibrate measures the loudness of the voice of each kind and source:
// the K-weighted RMS level (ITU-R BS.1770) over the span from the start
// that holds 90 per cent of the energy, for a voice at 0 dB. The gain of a
// kind is set from the mean level in decibels, so that a voice of unit
// scale on an engine whose output before the leveller has unit RMS level
// is at overLU plus popOffsetLU over the loudness of the engine, which is
// engineKDB over its RMS level. The voices have the short tail of the
// model (crackle.Params.ShortTail), so that the hold and the trailing
// shocks, which lengthen the span, do not move the gain and the loudness
// of the bang (docs/research/pop-tail.md). The sub-event counts of the
// draws cover the range of the kind in even steps, and each draw has its
// own seed. Only the requested crackle has other options than its own
// kind (shortCrackle), so the other requested kinds take the own gains.
// It allocates.
func (m *popModel) calibrate() {
	bank := m.bank.ShortTail()
	for r := range m.gain {
		for kind := range numKinds {
			if r == 1 && kind != kindCrackle {
				m.gain[r][kind] = m.gain[0][kind]
				continue
			}
			sh := &shapes[kind]
			var lg float64
			for d := range calDraws {
				subs := sh.subs + int(float64(sh.subsRange+1)*(float64(d)+0.5)/calDraws)
				if r == 1 {
					subs = min(subs, shortCrackle)
				}
				x := crackle.Render(bank, uint32(d+1), voiceOptions(m.params, kind, subs))
				lg += math.Log10(math.Max(spanLoudness(x, synthRate/100), 1e-12))
			}
			db := overLU[kind] + popOffsetLU + engineKDB + calTrimDB
			m.gain[r][kind] = float32(math.Pow(10, db/20-lg/calDraws))
		}
	}
}

// spanLoudness returns the K-weighted RMS level of x at the synthesiser
// rate over the span of spanLen.
func spanLoudness(x []float32, minN int) float64 {
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	n := spanLen(x, minN)
	if n == 0 {
		return 0
	}
	k := analysis.KWeighted(y, synthRate)
	var e float64
	for _, v := range k[:n] {
		e += v * v
	}
	return math.Sqrt(e / float64(n))
}

// spanLen returns the number of samples of x from its start to the sample
// at which the energy reaches 90 per cent of the total, and at least minN,
// or 0 when x has no energy.
func spanLen(x []float32, minN int) int {
	var tot float64
	for _, v := range x {
		tot += float64(v) * float64(v)
	}
	if tot == 0 {
		return 0
	}
	var e float64
	for i, v := range x {
		e += float64(v) * float64(v)
		if e >= 0.9*tot {
			return min(len(x), max(i+1, minN))
		}
	}
	return len(x)
}
