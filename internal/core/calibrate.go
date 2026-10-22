package core

import (
	"math"
	"sync"

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

// popModel is the crackle model of the pops at the synthesiser rate,
// which every engine shares and only reads: the parameters, the bank of
// the voices and the kind gains.
type popModel struct {
	params *crackle.Params
	bank   *crackle.Bank
	// gain is the gain of each kind that puts a voice of unit scale on an
	// engine of unit RMS level before the leveller at its overDB target,
	// for an own pop or a bang and for a requested pop.
	gain [2][numKinds]float32
}

// sharedModel returns the pop model of the process, or the error of the
// parameters of the crackle model. The first call makes the bank and
// calibrates the kind gains.
var sharedModel = sync.OnceValues(func() (*popModel, error) { return newPopModel(crackle.Fitted()) })

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

// calibrate measures the level of the voice of each kind and source: the
// RMS level over the span from the start that holds 90 per cent of the
// energy, for a voice at 0 dB. The gain of a kind is set from the mean
// level in decibels, so that a voice of unit scale on an engine whose
// output before the leveller has unit RMS level is at its overDB target.
// The sub-event counts of the draws cover the range of the kind in even
// steps, and each draw has its own seed. Only the requested crackle has
// other options than its own kind (shortCrackle), so the other requested
// kinds take the own gains. It allocates.
func (m *popModel) calibrate() {
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
				x := crackle.Render(m.bank, uint32(d+1), voiceOptions(m.params, kind, subs))
				lg += math.Log10(math.Max(spanRMS(x, synthRate/100), 1e-12))
			}
			m.gain[r][kind] = float32(math.Pow(10, (overDB[kind]+calTrimDB)/20-lg/calDraws))
		}
	}
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
			for _, u := range x[i+1 : n] {
				e += float64(u) * float64(u)
			}
			return math.Sqrt(e / float64(n))
		}
	}
	return 0
}
