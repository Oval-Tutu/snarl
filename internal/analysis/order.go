package analysis

import (
	"math"
)

// floorDB is the lowest level in decibels that the metrics use, so that
// silent bins do not produce infinite distances.
const floorDB = -120

// maxRevsPerFrame is the number of crank revolutions in one order FFT frame.
// Eight revolutions give a resolution of 0.125 orders, so half orders fall
// four bins apart.
const maxRevsPerFrame = 8

// Orders returns the engine orders from 0.5 to maxOrder in steps of 0.5.
func Orders(maxOrder float64) []float64 {
	var o []float64
	for k := 1; float64(k)*0.5 <= maxOrder+1e-9; k++ {
		o = append(o, float64(k)*0.5)
	}
	return o
}

// OrderSpectrum returns the level in dBFS of each engine order in x. The
// rpmAt function gives the RPM at t seconds after x[0]. The signal is
// resampled at constant crank-angle steps, so each order is a sharp peak even
// when the RPM changes. A sinusoid of amplitude A at an order gives
// 20*log10(A). It returns false when x spans fewer than two revolutions.
func OrderSpectrum(x []float64, rate int, rpmAt func(t float64) float64, orders []float64) (levels []float64, ok bool) {
	if len(x) < 2 {
		return nil, false
	}
	fs := float64(rate)
	phase := make([]float64, len(x))
	minRPM := math.Inf(1)
	prev := rpmAt(0)
	for i := 1; i < len(x); i++ {
		r := rpmAt(float64(i) / fs)
		phase[i] = phase[i-1] + (prev+r)/2/60/fs
		minRPM = min(minRPM, r, prev)
		prev = r
	}
	revs := phase[len(x)-1]
	nrev := maxRevsPerFrame
	for nrev > 2 && float64(nrev) > revs {
		nrev /= 2
	}
	if float64(nrev) > revs || minRPM <= 0 {
		return nil, false
	}
	// Samples per revolution at or above the input rate, so that the angular
	// resampling never decimates and needs no anti-alias filter.
	spr := nextPow2(int(math.Ceil(fs * 60 / minRPM)))
	angular := resampleByPhase(x, phase, spr)
	n := spr * nrev
	if len(angular) < n {
		return nil, false
	}
	sp := newSpectrum(n)
	p := make([]float64, n/2+1)
	acc := make([]float64, n/2+1)
	frames := 0
	for start := 0; start+n <= len(angular); start += n / 2 {
		sp.power(angular[start:start+n], p)
		for k, v := range p {
			acc[k] += v
		}
		frames++
	}
	var wsum float64
	for _, w := range sp.window {
		wsum += w
	}
	// A sinusoid of amplitude A gives a peak power of (A*wsum/2)^2, and the
	// Hann main lobe adds a quarter of that in each neighbouring bin.
	norm := float64(frames) * 1.5 * wsum * wsum / 4
	levels = make([]float64, len(orders))
	for i, o := range orders {
		c := int(math.Round(o * float64(nrev)))
		var sum float64
		for k := c - 1; k <= c+1; k++ {
			if k >= 0 && k < len(acc) {
				sum += acc[k]
			}
		}
		levels[i] = powerDB(sum / norm)
	}
	return levels, true
}

// resampleByPhase interpolates x at spr equal steps per revolution of the
// monotonic phase track, in revolutions.
func resampleByPhase(x, phase []float64, spr int) []float64 {
	m := int(phase[len(phase)-1] * float64(spr))
	out := make([]float64, m)
	i := 0
	for k := range out {
		target := float64(k) / float64(spr)
		for i+1 < len(phase)-1 && phase[i+1] <= target {
			i++
		}
		d := phase[i+1] - phase[i]
		f := 0.0
		if d > 0 {
			f = (target - phase[i]) / d
		}
		out[k] = x[i] + (x[i+1]-x[i])*f
	}
	return out
}

// DominantOrder returns the order with the highest level.
func DominantOrder(orders, levels []float64) float64 {
	best := 0
	for i, v := range levels {
		if v > levels[best] {
			best = i
		}
	}
	return orders[best]
}

// RMSDifferenceDB returns the root mean square difference of two level
// vectors in decibels.
func RMSDifferenceDB(a, b []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(a)))
}

func powerDB(p float64) float64 {
	if p <= 0 {
		return floorDB
	}
	return max(10*math.Log10(p), floorDB)
}
