package model

import (
	"math"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// formants is a cascade of biquads with one gain: the formant bank of the
// engine, which takes the place of the engine-sim impulse response, or
// the pop bank of one path. tools/fit fits the sections. A bank with no
// section passes its input with the gain.
type formants struct {
	g  float32
	bq []dsp.Biquad
}

// newFormants designs the bank at the sample rate, with the coefficients
// in float64.
func newFormants(fs []FormantSpec, gainDB, rate float64) formants {
	f := formants{g: float32(math.Pow(10, gainDB/20)), bq: make([]dsp.Biquad, len(fs))}
	for i, s := range fs {
		switch s.Kind {
		case "lowpass":
			f.bq[i].SetLowPass(s.Hz, s.Q, rate)
		case "highpass":
			f.bq[i].SetHighPass(s.Hz, s.Q, rate)
		default:
			f.bq[i].SetPeak(s.Hz, s.Q, s.GainDB, rate)
		}
	}
	return f
}

// process filters one sample.
func (f *formants) process(x float32) float32 {
	x *= f.g
	for i := range f.bq {
		x = f.bq[i].Process(x)
	}
	return x
}

// reset clears the state of every section.
func (f *formants) reset() {
	for i := range f.bq {
		f.bq[i].Reset()
	}
}
