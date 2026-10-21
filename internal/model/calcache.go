package model

import "sync"

// calCacheMax is the most calibrations that the cache holds. A program
// that makes more distinct specs than this calibrates again for the
// specs that the cache dropped.
const calCacheMax = 64

// calKey holds every input of pops.calibrate: the exhaust paths, the
// shared exhaust settings, the pop bank, the pop shape and the sample
// rate. The pop settings that only the triggers and the gains of the own
// and requested pops read, and the exhaust settings of the engine sound
// only, are zero, so that specs that differ only in them share one
// calibration. TestCalibrationCache checks that these settings do not
// change the calibration.
type calKey struct {
	paths   [MaxBanks]PathSpec
	n       int
	exhaust ExhaustSpec
	bank    [MaxFormants]FormantSpec
	nBank   int
	pops    PopSpec
	rate    float64
}

func newCalKey(s *Spec, rate float64) calKey {
	k := calKey{n: len(s.Paths), exhaust: s.Exhaust, nBank: len(s.PopFormants), pops: s.Pops, rate: rate}
	copy(k.paths[:], s.Paths)
	copy(k.bank[:], s.PopFormants)
	k.pops.Density, k.pops.Character, k.pops.Gain = 0, "", 0
	k.pops.WeightPop, k.pops.WeightBoom, k.pops.WeightCrackle, k.pops.WeightBurble = 0, 0, 0, 0
	k.exhaust.Crossover, k.exhaust.Coupling, k.exhaust.FormantGainDB = "", 0, 0
	return k
}

// calEntry is one calibration. once runs the calibration, so that
// concurrent calls of New for one spec calibrate once.
type calEntry struct {
	once sync.Once
	gain [MaxBanks][2][numKinds]float32
}

// calCache holds the kind gains of each calibrated spec and sample rate
// for the life of the process, so that New calibrates each profile once.
var calCache struct {
	sync.Mutex
	m map[calKey]*calEntry
}

// calibrateCached sets the kind gains of p from the cache, and runs
// calibrate for a spec and sample rate that the cache does not hold.
func (p *pops) calibrateCached(s *Spec, rate float64) {
	k := newCalKey(s, rate)
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
	e.once.Do(func() {
		c := *p
		c.calibrate(s, rate)
		e.gain = c.kindGain
	})
	p.kindGain = e.gain
}
