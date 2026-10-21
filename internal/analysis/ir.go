package analysis

import "math"

// The impulse response measurement describes an engine-sim impulse
// response as the synthesiser uses it, and how far a shorter FIR filter or
// an all-pole IIR filter is from it.

const (
	// irThreshold is the level below which engine-sim trims the tail of
	// an impulse response: the last sample with a magnitude above 100 in
	// 16-bit units ends it.
	irThreshold = 100.0 / 32768
	// irFFT is the zero-padded transform size of a response. At 44.1 kHz,
	// a bin is 0.67 Hz wide, so every third-octave band from 25 Hz holds
	// several bins.
	irFFT = 1 << 16
)

// IRTrim returns ir as engine-sim loads it: up to the last sample with a
// magnitude above irThreshold, and at most maxTaps samples.
func IRTrim(ir []float64, maxTaps int) []float64 {
	n := 0
	for i, v := range ir {
		if math.Abs(v) > irThreshold {
			n = i + 1
		}
	}
	return ir[:min(n, maxTaps)]
}

// IRResponse returns the power response of the FIR filter ir at rate in
// each band, in dB: the mean of |H|^2 over the bins of the band. A band
// with no bin gives floorDB.
func IRResponse(ir []float64, rate int, bands [][2]float64) []float64 {
	n := max(irFFT, nextPow2(len(ir)))
	buf := make([]complex128, n)
	for i, v := range ir {
		buf[i] = complex(v, 0)
	}
	FFT(buf)
	return bandMeans(buf, n, rate, bands)
}

// SynthPathResponse returns the power response in dB per band of the
// harmonic path of the engine-sim synthesiser around the impulse response
// ir at rate: the fourth-order Butterworth low pass at lowPassHz on the
// input (as its analogue magnitude), the first difference that the
// derivative mix takes, and the convolution with ir. The level is that of
// a derivative in units per sample, so that only the shape counts.
func SynthPathResponse(ir []float64, rate int, lowPassHz float64, bands [][2]float64) []float64 {
	n := max(irFFT, nextPow2(len(ir)))
	buf := make([]complex128, n)
	for i, v := range ir {
		buf[i] = complex(v, 0)
	}
	FFT(buf)
	for k := range buf {
		f := float64(min(k, n-k)) * float64(rate) / float64(n)
		d := 2 * math.Sin(math.Pi*f/float64(rate))
		l := 1 / math.Sqrt(1+math.Pow(f/lowPassHz, 8))
		buf[k] *= complex(d*l, 0)
	}
	return bandMeans(buf, n, rate, bands)
}

// bandMeans returns the mean of |X|^2 over the bins of each band, in dB,
// for the transform X of n points at rate.
func bandMeans(x []complex128, n, rate int, bands [][2]float64) []float64 {
	binHz := float64(rate) / float64(n)
	out := make([]float64, len(bands))
	for i, b := range bands {
		var sum float64
		bins := 0
		for k := int(math.Ceil(b[0] / binHz)); k <= n/2 && float64(k)*binHz < b[1]; k++ {
			re, im := real(x[k]), imag(x[k])
			sum += re*re + im*im
			bins++
		}
		out[i] = floorDB
		if bins > 0 {
			out[i] = powerDB(sum / float64(bins))
		}
	}
	return out
}

// AllPole returns the all-pole model g/A(z) of order p of ir, by linear
// prediction with the autocorrelation method and the Levinson-Durbin
// recursion: a has p+1 coefficients with a[0] = 1, and g^2 is the
// prediction error energy, so that the model has about the energy of ir.
// It returns false when ir is silent or shorter than p+1 samples.
func AllPole(ir []float64, p int) (a []float64, g float64, ok bool) {
	if p < 1 || len(ir) <= p {
		return nil, 0, false
	}
	r := make([]float64, p+1)
	for lag := range r {
		for i := lag; i < len(ir); i++ {
			r[lag] += ir[i] * ir[i-lag]
		}
	}
	if r[0] <= 0 {
		return nil, 0, false
	}
	a = make([]float64, p+1)
	a[0] = 1
	e := r[0]
	prev := make([]float64, p+1)
	for i := 1; i <= p; i++ {
		acc := r[i]
		for j := 1; j < i; j++ {
			acc += a[j] * r[i-j]
		}
		k := -acc / e
		copy(prev, a)
		for j := 1; j < i; j++ {
			a[j] = prev[j] + k*prev[i-j]
		}
		a[i] = k
		e *= 1 - k*k
		if e <= 0 {
			return nil, 0, false
		}
	}
	return a, math.Sqrt(e), true
}

// AllPoleResponse returns the power response of g/A(z) at rate in each
// band, in dB, as IRResponse does for a FIR filter.
func AllPoleResponse(a []float64, g float64, rate int, bands [][2]float64) []float64 {
	buf := make([]complex128, irFFT)
	for i, v := range a {
		buf[i] = complex(v, 0)
	}
	FFT(buf)
	for k, v := range buf {
		re, im := real(v), imag(v)
		d := re*re + im*im
		if d > 0 {
			buf[k] = complex(g/math.Sqrt(d), 0)
		}
	}
	return bandMeans(buf, irFFT, rate, bands)
}

// EnergyDecay returns the times in seconds at which the energy of ir that
// is still to come, integrated backwards from the end (Schroeder), falls
// 20 dB and 40 dB below the total energy. A level that the tail never
// reaches gives the length of ir.
func EnergyDecay(ir []float64, rate int) (t20, t40 float64) {
	var total float64
	for _, v := range ir {
		total += v * v
	}
	t20, t40 = float64(len(ir))/float64(rate), float64(len(ir))/float64(rate)
	if total <= 0 {
		return t20, t40
	}
	rest := total
	found20 := false
	for i, v := range ir {
		rest -= v * v
		t := float64(i+1) / float64(rate)
		if !found20 && rest <= total*1e-2 {
			t20, found20 = t, true
		}
		if rest <= total*1e-4 {
			return t20, t
		}
	}
	return t20, t40
}
