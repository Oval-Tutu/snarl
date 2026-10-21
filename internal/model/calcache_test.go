package model

import (
	"sync"
	"testing"
)

// freshGains returns the kind gains of a calibration of s that does not
// use the cache.
func freshGains(s *Spec) [MaxBanks][2][numKinds]float32 {
	p := newPops(s, rate, chunk/float64(rate))
	p.calibrate(s, rate)
	return p.kindGain
}

// twoPathSpec is a four-cylinder engine with two exhaust paths of
// different lengths and no crossover.
func twoPathSpec() Spec {
	s := limiterSpec("ignition")
	s.Cylinders[1].Bank, s.Cylinders[3].Bank = 1, 1
	q := s.Paths[0]
	q.CollectorM, q.Gain = 0.9, 0.8
	s.Paths = append(s.Paths, q)
	return s
}

// TestCalibrationCache checks that New takes the kind gains from the cache
// and that they equal a fresh calibration, for one and two paths. Specs
// that differ only in the pop density, character, gain or kind weights, or
// in the crossover, overlap or formant gain, share one calibration, and specs that differ in a pop shape value, a
// path or the sample rate do not.
func TestCalibrationCache(t *testing.T) {
	for _, s := range []Spec{popSpec(), twoPathSpec()} {
		want := freshGains(&s)
		y, err := New(&s, rate)
		if err != nil {
			t.Fatal(err)
		}
		if y.pops.kindGain != want {
			t.Errorf("%d paths: cached gains %v, want %v", len(s.Paths), y.pops.kindGain, want)
		}
		v := s
		v.Pops.Density, v.Pops.Character, v.Pops.Gain = 3, "race", 0.5
		v.Pops.WeightPop, v.Pops.WeightBoom, v.Pops.WeightCrackle, v.Pops.WeightBurble = 0, 1, 0, 0
		if g := freshGains(&v); g != want {
			t.Errorf("%d paths: the density, character, gain and weights change the calibration", len(s.Paths))
		}
		if newCalKey(&v, rate) != newCalKey(&s, rate) {
			t.Errorf("%d paths: the density, character, gain and weights change the cache key", len(s.Paths))
		}
		v = s
		v.Exhaust.Crossover, v.Exhaust.Coupling, v.Exhaust.Overlap, v.Exhaust.FormantGainDB = "partial", 0.3, 0.5, -6
		if g := freshGains(&v); g != want {
			t.Errorf("%d paths: the crossover, overlap and formant gain change the calibration", len(s.Paths))
		}
		if newCalKey(&v, rate) != newCalKey(&s, rate) {
			t.Errorf("%d paths: the crossover, overlap and formant gain change the cache key", len(s.Paths))
		}
		for name, change := range map[string]func(*Spec){
			"body_hz": func(q *Spec) { q.Pops.BodyHz *= 1.1 },
			"path":    func(q *Spec) { q.Paths = append([]PathSpec{}, q.Paths...); q.Paths[0].CollectorM *= 1.1 },
			"exhaust": func(q *Spec) { q.Exhaust.SpeedOfSound *= 1.1 },
			"pop bank": func(q *Spec) {
				q.PopFormants = append([]FormantSpec{}, q.PopFormants...)
				q.PopFormants[0].GainDB++
			},
		} {
			q := s
			change(&q)
			if newCalKey(&q, rate) == newCalKey(&s, rate) {
				t.Errorf("%d paths: a change of %s keeps the cache key", len(s.Paths), name)
			}
		}
		if newCalKey(&s, 44100) == newCalKey(&s, rate) {
			t.Errorf("%d paths: the sample rate does not change the cache key", len(s.Paths))
		}
	}
}

// TestCalibrationConcurrent checks that concurrent calls of New for a new
// spec give the gains of a fresh calibration.
func TestCalibrationConcurrent(t *testing.T) {
	s := twoPathSpec()
	s.Pops.BodyHz *= 1.05 // A spec that no other test calibrates.
	want := freshGains(&s)
	var wg sync.WaitGroup
	got := make([][MaxBanks][2][numKinds]float32, 8)
	for i := range got {
		wg.Go(func() {
			q := s
			y, err := New(&q, rate)
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = y.pops.kindGain
		})
	}
	wg.Wait()
	for i, g := range got {
		if g != want {
			t.Errorf("call %d: gains %v, want %v", i, g, want)
		}
	}
}
