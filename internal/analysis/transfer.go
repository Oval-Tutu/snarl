package analysis

import (
	"math"
	"math/cmplx"
)

// The transfer measurement compares the input of a system with its output:
// for engine-sim, the exhaust pressure that feeds the synthesiser with the
// synthesiser output. It estimates the linear transfer function with the
// Welch H1 estimator, the cross spectrum over the input auto spectrum, and
// splits the output into the part that the input explains (coherent) and
// the rest (noise that the system adds, and nonlinear products).

// TransferBand is the transfer measurement in one band.
type TransferBand struct {
	Lo float64 `json:"lo_hz"`
	Hi float64 `json:"hi_hz"`
	// GainDB is the coherent output power over the input power in the
	// band: the power-weighted mean of |H1|^2.
	GainDB float64 `json:"gain_db"`
	// NoiseDB is the output power that the input does not explain over
	// the output power in the band.
	NoiseDB float64 `json:"noise_db"`
	// OutputDB is the output power in the band over the output power of
	// the whole spectrum.
	OutputDB float64 `json:"output_db"`
	// Bins is the number of FFT bins in the band. A band with no bin has
	// no measurement.
	Bins int `json:"bins"`
}

// TransferResult is the transfer measurement of one stretch of input and
// output.
type TransferResult struct {
	// Lag is the delay of the output behind the input in samples, which
	// the measurement removes first.
	Lag int `json:"lag_samples"`
	// InputDB and OutputDB are the mean squares of the input and the
	// output in dB.
	InputDB  float64 `json:"input_db"`
	OutputDB float64 `json:"output_db"`
	// GainDB is OutputDB minus InputDB.
	GainDB float64 `json:"gain_db"`
	// NoiseDB is the output power that the input does not explain over
	// the whole output power.
	NoiseDB float64 `json:"noise_db"`
	// Frames is the number of averaged frames. An output with no relation
	// to the input gives a NoiseDB of about -10*log10(1-1/Frames), not 0.
	Frames int            `json:"frames"`
	Bands  []TransferBand `json:"bands"`
}

// crossSpectra returns the Welch auto spectra of x and y and their cross
// spectrum conj(X)*Y for the bins 0 to n/2, over Hann frames of n samples
// with half overlap, and the number of frames.
func crossSpectra(x, y []float64, n int) (sxx, syy []float64, sxy []complex128, frames int) {
	w := Hann(n)
	bx := make([]complex128, n)
	by := make([]complex128, n)
	sxx = make([]float64, n/2+1)
	syy = make([]float64, n/2+1)
	sxy = make([]complex128, n/2+1)
	for start := 0; start+n <= min(len(x), len(y)); start += n / 2 {
		for i := range n {
			bx[i] = complex(x[start+i]*w[i], 0)
			by[i] = complex(y[start+i]*w[i], 0)
		}
		FFT(bx)
		FFT(by)
		for k := range sxx {
			sxx[k] += real(bx[k])*real(bx[k]) + imag(bx[k])*imag(bx[k])
			syy[k] += real(by[k])*real(by[k]) + imag(by[k])*imag(by[k])
			sxy[k] += cmplx.Conj(bx[k]) * by[k]
		}
		frames++
	}
	return sxx, syy, sxy, frames
}

// Lag returns the delay of y behind x in samples, from -maxLag to maxLag,
// that lets x explain the most of y: the lag with the least NoiseDB of
// Transfer over Hann frames of size samples. It searches in steps of step
// samples, then in single samples around the best step. The coherence
// rates the fit of the whole spectrum, so that it also finds the lag of a
// periodic signal, or of an output that a long impulse response spreads,
// where the peak of a cross correlation is ambiguous. It returns false
// when the inputs are shorter than four frames plus maxLag.
func Lag(x, y []float64, rate, size, maxLag, step int) (int, bool) {
	n := min(len(x), len(y))
	if size < 4 || size&(size-1) != 0 || maxLag < 0 || n < 4*size+maxLag {
		return 0, false
	}
	step = max(step, 1)
	noise := func(lag int) float64 {
		r, ok := Transfer(x, y, rate, size, lag, nil)
		if !ok {
			return math.Inf(1)
		}
		return r.NoiseDB
	}
	best, bestN := 0, math.Inf(1)
	for lag := -maxLag; lag <= maxLag; lag += step {
		if v := noise(lag); v < bestN {
			best, bestN = lag, v
		}
	}
	coarse := best
	for lag := max(coarse-step+1, -maxLag); lag <= min(coarse+step-1, maxLag); lag++ {
		if v := noise(lag); v < bestN {
			best, bestN = lag, v
		}
	}
	return best, true
}

// Transfer measures the transfer from the input x to the output y, both
// at rate, with Hann frames of size samples (a power of two), with y lag
// samples behind x (Lag measures it). It returns false when the inputs are
// shorter than four frames or silent.
func Transfer(x, y []float64, rate, size, lag int, bands [][2]float64) (TransferResult, bool) {
	n := min(len(x), len(y))
	if size < 4 || size&(size-1) != 0 || n < 4*size+max(lag, -lag) {
		return TransferResult{}, false
	}
	xs, ys := x[:n], y[:n]
	if lag > 0 {
		xs, ys = x[:n-lag], y[lag:n]
	} else if lag < 0 {
		xs, ys = x[-lag:n], y[:n+lag]
	}
	sxx, syy, sxy, frames := crossSpectra(xs, ys, size)
	var px, py float64
	for _, v := range xs {
		px += v * v
	}
	for _, v := range ys {
		py += v * v
	}
	if px <= 0 || py <= 0 {
		return TransferResult{}, false
	}
	px /= float64(len(xs))
	py /= float64(len(ys))
	coherent := make([]float64, len(sxx))
	var tc, ty float64
	for k := 1; k < len(sxx); k++ {
		if sxx[k] > 0 {
			a := cmplx.Abs(sxy[k])
			coherent[k] = min(a*a/sxx[k], syy[k])
		}
		tc += coherent[k]
		ty += syy[k]
	}
	res := TransferResult{
		Lag:      lag,
		InputDB:  powerDB(px),
		OutputDB: powerDB(py),
		NoiseDB:  powerRatioDB(ty-tc, ty),
		Frames:   frames,
	}
	res.GainDB = res.OutputDB - res.InputDB
	binHz := float64(rate) / float64(size)
	for _, b := range bands {
		var c, xx, yy float64
		bins := 0
		for k := 1; k < len(sxx); k++ {
			if f := float64(k) * binHz; f >= b[0] && f < b[1] {
				c += coherent[k]
				xx += sxx[k]
				yy += syy[k]
				bins++
			}
		}
		res.Bands = append(res.Bands, TransferBand{
			Lo:       b[0],
			Hi:       b[1],
			Bins:     bins,
			GainDB:   powerRatioDB(c, xx),
			NoiseDB:  powerRatioDB(yy-c, yy),
			OutputDB: powerRatioDB(yy, ty),
		})
	}
	return res, true
}

// ThirdOctaveBands returns the edges in hertz of the third-octave bands
// from 25 Hz whose upper edge is below the Nyquist frequency of rate.
func ThirdOctaveBands(rate int) [][2]float64 {
	var out [][2]float64
	for k := -16; k <= 13; k++ {
		fc := 1000 * math.Pow(10, float64(k)/10)
		lo, hi := fc*math.Pow(10, -0.05), fc*math.Pow(10, 0.05)
		if hi > float64(rate)/2 {
			break
		}
		out = append(out, [2]float64{lo, hi})
	}
	return out
}
