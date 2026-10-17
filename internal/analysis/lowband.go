package analysis

import (
	"math"
	"slices"
)

// Default edges in hertz of the low band.
const (
	DefaultBandLo = 20
	DefaultBandHi = 300
)

// bandPreRoll is the audio in seconds before a window that BandPass filters
// and drops, so that the filter has settled when the window starts.
const bandPreRoll = 0.2

// cycleSamples is the number of crank-angle samples in one engine cycle of
// 720 degrees, so one sample is half a degree.
const cycleSamples = 1440

// minCycles is the fewest whole engine cycles that CycleVariation measures.
const minCycles = 3

// LowBandOrder is the order spectrum restricted to the low band.
type LowBandOrder struct {
	// Orders are the orders whose frequency at the mean window RPM is in the
	// band.
	Orders []float64 `json:"orders"`
	// Distance is the root mean square level difference of those orders.
	Distance float64 `json:"distance_db"`
	// HalfOrderDB is the power of the half orders (0.5, 1.5, ...) relative
	// to the power of the whole orders, both in the band. It is nil when the
	// band holds no half order or no whole order.
	HalfOrderDB *Pair `json:"half_order_db,omitempty"`
}

// CycleMetrics measures how the low band changes from one engine cycle to
// the next.
type CycleMetrics struct {
	// Cycles is the number of whole engine cycles measured in A and B.
	Cycles [2]int `json:"cycles"`
	// EnergyCV is the coefficient of variation of the per-cycle energy.
	EnergyCV Pair `json:"energy_cv"`
	// Correlation is the mean normalised correlation of consecutive cycles.
	Correlation Pair `json:"correlation"`
}

// LowBandMetrics are the metrics of one window in the low band.
type LowBandMetrics struct {
	Order          *LowBandOrder `json:"order,omitempty"`
	ThirdOctaveLSD float64       `json:"third_octave_lsd_db"`
	BalanceDB      Pair          `json:"balance_db"`
	Cycles         *CycleMetrics `json:"cycles,omitempty"`
}

// BandOrders returns the indices of the orders whose frequency at rpm is from
// lo to hi hertz.
func BandOrders(orders []float64, rpm, lo, hi float64) []int {
	var idx []int
	for i, o := range orders {
		if f := o * rpm / 60; f >= lo && f <= hi {
			idx = append(idx, i)
		}
	}
	return idx
}

// HalfOrderDB returns the summed power of the half orders among idx relative
// to the summed power of the whole orders among idx, in dB. It returns false
// when idx holds no half order or no whole order.
func HalfOrderDB(orders, levels []float64, idx []int) (float64, bool) {
	var half, whole float64
	var nh, nw int
	for _, i := range idx {
		p := math.Pow(10, levels[i]/10)
		if math.Mod(orders[i], 1) == 0 {
			whole += p
			nw++
		} else {
			half += p
			nh++
		}
	}
	if nh == 0 || nw == 0 {
		return 0, false
	}
	return powerDB(half) - powerDB(whole), true
}

// BandBalanceDB returns the power from lo to hi hertz relative to the power
// of the whole spectrum above 0 Hz, in dB.
func BandBalanceDB(x []float64, rate int, lo, hi float64) float64 {
	p, n := welch(x, thirdOctaveFFT)
	if p == nil {
		return 0
	}
	binHz := float64(rate) / float64(n)
	var band, total float64
	for k := 1; k < len(p); k++ {
		total += p[k]
		if f := float64(k) * binHz; f >= lo && f <= hi {
			band += p[k]
		}
	}
	if total == 0 {
		return 0
	}
	return powerDB(band / total)
}

// bandLevels returns the third-octave levels of x whose centre frequency is
// from lo to hi hertz.
func bandLevels(x []float64, rate int, lo, hi float64) []float64 {
	centres, levels := ThirdOctaveLevels(x, rate)
	var out []float64
	for i, fc := range centres {
		if fc >= lo && fc <= hi {
			out = append(out, levels[i])
		}
	}
	return out
}

// BandPass filters x with a fourth-order Butterworth high-pass at lo hertz
// followed by a fourth-order Butterworth low-pass at hi hertz.
func BandPass(x []float64, rate int, lo, hi float64) []float64 {
	y := x
	for _, q := range []float64{0.5412, 1.3066} {
		y = rbj(float64(rate), lo, q, true).apply(y)
		y = rbj(float64(rate), hi, q, false).apply(y)
	}
	return y
}

// rbj returns an RBJ cookbook high-pass or low-pass biquad.
func rbj(fs, f0, q float64, high bool) biquad {
	s, c := math.Sincos(2 * math.Pi * f0 / fs)
	alpha := s / (2 * q)
	b0, b1 := (1-c)/2, 1-c
	if high {
		b0, b1 = (1+c)/2, -(1 + c)
	}
	a0 := 1 + alpha
	return biquad{b0: b0 / a0, b1: b1 / a0, b2: b0 / a0, a1: -2 * c / a0, a2: (1 - alpha) / a0}
}

// CycleVariation measures the change of x from one engine cycle of 720 crank
// degrees to the next. The rpmAt function gives the RPM at t seconds after
// x[0]. It resamples x to cycleSamples steps per cycle and splits it into
// whole cycles from x[0]. The energy CV is the population standard deviation
// of the per-cycle energy divided by its mean: 0 when every cycle is equally
// loud. The correlation is the mean over consecutive pairs of
// sum(c[k]*c[k+1]) / sqrt(sum(c[k]^2) * sum(c[k+1]^2)) at zero lag: 1 when
// the waveform repeats exactly, lower when it changes. It returns false when
// x holds fewer than three whole cycles.
func CycleVariation(x []float64, rate int, rpmAt func(t float64) float64) (energyCV, correlation float64, cycles int, ok bool) {
	if len(x) < 2 {
		return 0, 0, 0, false
	}
	phase, minRPM := crankPhase(len(x), rate, rpmAt)
	if minRPM <= 0 {
		return 0, 0, 0, false
	}
	angular := resampleByPhase(x, phase, cycleSamples/2)
	cycles = len(angular) / cycleSamples
	if cycles < minCycles {
		return 0, 0, cycles, false
	}
	energy := make([]float64, cycles)
	for k := range energy {
		for _, v := range angular[k*cycleSamples : (k+1)*cycleSamples] {
			energy[k] += v * v
		}
	}
	var mean, varSum, corr float64
	for _, e := range energy {
		mean += e
	}
	mean /= float64(cycles)
	for _, e := range energy {
		varSum += (e - mean) * (e - mean)
	}
	if mean > 0 {
		energyCV = math.Sqrt(varSum/float64(cycles)) / mean
	}
	for k := range cycles - 1 {
		c0 := angular[k*cycleSamples : (k+1)*cycleSamples]
		c1 := angular[(k+1)*cycleSamples : (k+2)*cycleSamples]
		var dot float64
		for i := range c0 {
			dot += c0[i] * c1[i]
		}
		if d := math.Sqrt(energy[k] * energy[k+1]); d > 0 {
			corr += dot / d
		}
	}
	return energyCV, corr / float64(cycles-1), cycles, true
}

// lowBand computes the low-band metrics of a window that spans a[i0:i1] and
// b[i0:i1]. The order spectra oa and ob are nil when the window has none.
func lowBand(a, b []float64, i0, i1, rate int, w *Window, oa, ob []float64, opts *Options) *LowBandMetrics {
	lo, hi := opts.BandLo, opts.BandHi
	xa, xb := a[i0:i1], b[i0:i1]
	m := &LowBandMetrics{
		ThirdOctaveLSD: RMSDifferenceDB(bandLevels(xa, rate, lo, hi), bandLevels(xb, rate, lo, hi)),
		BalanceDB:      newPair(BandBalanceDB(xa, rate, lo, hi), BandBalanceDB(xb, rate, lo, hi)),
	}
	if w.rpmAt == nil {
		return m
	}
	if oa != nil && ob != nil {
		idx := BandOrders(opts.Orders, (w.RPMStart+w.RPMEnd)/2, lo, hi)
		bo := &LowBandOrder{Orders: []float64{}}
		va, vb := make([]float64, len(idx)), make([]float64, len(idx))
		for j, i := range idx {
			bo.Orders = append(bo.Orders, opts.Orders[i])
			va[j], vb[j] = oa[i], ob[i]
		}
		bo.Distance = RMSDifferenceDB(va, vb)
		ha, okA := HalfOrderDB(opts.Orders, oa, idx)
		hb, okB := HalfOrderDB(opts.Orders, ob, idx)
		if okA && okB {
			p := newPair(ha, hb)
			bo.HalfOrderDB = &p
		}
		m.Order = bo
	}
	pre := min(i0, int(bandPreRoll*float64(rate)))
	rpm := func(t float64) float64 { return w.rpmAt(w.Start + t) }
	fa := BandPass(a[i0-pre:i1], rate, lo, hi)[pre:]
	fb := BandPass(b[i0-pre:i1], rate, lo, hi)[pre:]
	cvA, rA, nA, okA := CycleVariation(fa, rate, rpm)
	cvB, rB, nB, okB := CycleVariation(fb, rate, rpm)
	if okA && okB {
		m.Cycles = &CycleMetrics{
			Cycles:      [2]int{nA, nB},
			EnergyCV:    newPair(cvA, cvB),
			Correlation: newPair(rA, rB),
		}
	}
	return m
}

// LowBandSummary holds the median of each low-band metric over the windows
// that have it.
type LowBandSummary struct {
	OrderDistance  *float64 `json:"order_distance_db,omitempty"`
	ThirdOctaveLSD *float64 `json:"third_octave_lsd_db,omitempty"`
	BalanceDB      *Pair    `json:"balance_db,omitempty"`
	EnergyCV       *Pair    `json:"energy_cv,omitempty"`
	Correlation    *Pair    `json:"correlation,omitempty"`
	HalfOrderDB    *Pair    `json:"half_order_db,omitempty"`
}

// SummariseLowBand returns the medians of the low-band metrics in ms.
func SummariseLowBand(ms []WindowMetrics) LowBandSummary {
	var od, lsd, balA, balB, cvA, cvB, rA, rB, hA, hB []float64
	for i := range ms {
		l := ms[i].LowBand
		if l == nil {
			continue
		}
		lsd = append(lsd, l.ThirdOctaveLSD)
		balA, balB = append(balA, l.BalanceDB.A), append(balB, l.BalanceDB.B)
		if l.Order != nil {
			od = append(od, l.Order.Distance)
			if h := l.Order.HalfOrderDB; h != nil {
				hA, hB = append(hA, h.A), append(hB, h.B)
			}
		}
		if c := l.Cycles; c != nil {
			cvA, cvB = append(cvA, c.EnergyCV.A), append(cvB, c.EnergyCV.B)
			rA, rB = append(rA, c.Correlation.A), append(rB, c.Correlation.B)
		}
	}
	var s LowBandSummary
	if len(od) > 0 {
		v := median(od)
		s.OrderDistance = &v
	}
	if len(lsd) > 0 {
		v := median(lsd)
		s.ThirdOctaveLSD = &v
	}
	s.BalanceDB = medianPair(balA, balB)
	s.EnergyCV = medianPair(cvA, cvB)
	s.Correlation = medianPair(rA, rB)
	s.HalfOrderDB = medianPair(hA, hB)
	return s
}

func medianPair(a, b []float64) *Pair {
	if len(a) == 0 {
		return nil
	}
	p := newPair(median(a), median(b))
	return &p
}

func median(x []float64) float64 {
	s := slices.Clone(x)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
