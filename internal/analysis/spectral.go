package analysis

import (
	"math"
)

const (
	thirdOctaveFFT = 8192
	fluxFFT        = 2048
	fluxHop        = 512
	logMagEps      = 1e-7
)

// DefaultFFTSizes are the multi-resolution STFT sizes. They span long tones
// down to pulse structure under 2 ms at 48 kHz.
var DefaultFFTSizes = []int{4096, 2048, 1024, 512, 256, 128, 64}

// welch returns the mean power spectrum of x over Hann frames of size n with
// half overlap. It shortens n to fit x.
func welch(x []float64, n int) (p []float64, size int) {
	for n > 64 && n > len(x) {
		n /= 2
	}
	if n > len(x) {
		return nil, n
	}
	sp := newSpectrum(n)
	frame := make([]float64, n/2+1)
	p = make([]float64, n/2+1)
	frames := 0
	for start := 0; start+n <= len(x); start += n / 2 {
		sp.power(x[start:start+n], frame)
		for k, v := range frame {
			p[k] += v
		}
		frames++
	}
	for k := range p {
		p[k] /= float64(frames)
	}
	return p, n
}

// ThirdOctaveLevels returns the long-term power in dB of each third-octave
// band from 25 Hz to 16 kHz that lies below the Nyquist frequency and holds
// at least one FFT bin.
func ThirdOctaveLevels(x []float64, rate int) (centres, levels []float64) {
	p, n := welch(x, thirdOctaveFFT)
	if p == nil {
		return nil, nil
	}
	binHz := float64(rate) / float64(n)
	for k := -16; k <= 12; k++ {
		fc := 1000 * math.Pow(10, float64(k)/10)
		lo, hi := fc*math.Pow(10, -0.05), fc*math.Pow(10, 0.05)
		if hi > float64(rate)/2 {
			break
		}
		var sum float64
		bins := 0
		for b := int(math.Ceil(lo / binHz)); float64(b)*binHz < hi && b < len(p); b++ {
			sum += p[b]
			bins++
		}
		if bins == 0 {
			continue
		}
		centres = append(centres, fc)
		levels = append(levels, powerDB(sum/float64(n*n)))
	}
	return centres, levels
}

// MRSTFTSize is the multi-resolution STFT distance at one FFT size.
type MRSTFTSize struct {
	FFT                 int     `json:"fft"`
	SpectralConvergence float64 `json:"spectral_convergence"`
	LogMagnitude        float64 `json:"log_magnitude"`
}

// MRSTFTResult is the multi-resolution STFT distance. Total is the mean over
// sizes of spectral convergence plus log magnitude distance.
type MRSTFTResult struct {
	Total               float64      `json:"total"`
	SpectralConvergence float64      `json:"spectral_convergence"`
	LogMagnitude        float64      `json:"log_magnitude"`
	Sizes               []MRSTFTSize `json:"sizes"`
}

// MRSTFT returns the multi-resolution STFT distance of b from a. Each size
// uses a Hann window and 75 per cent overlap. Spectral convergence is
// ||A|-|B||_F / ||A||_F. Log magnitude is the mean absolute difference of the
// natural logarithm of the magnitudes. Sizes longer than the input are
// skipped.
func MRSTFT(a, b []float64, sizes []int) MRSTFTResult {
	n := min(len(a), len(b))
	var r MRSTFTResult
	for _, size := range sizes {
		if size > n {
			continue
		}
		sp := newSpectrum(size)
		ma := make([]float64, size/2+1)
		mb := make([]float64, size/2+1)
		var diff2, ref2, logSum float64
		count := 0
		for start := 0; start+size <= n; start += size / 4 {
			sp.magnitude(a[start:start+size], ma)
			sp.magnitude(b[start:start+size], mb)
			for k := range ma {
				d := ma[k] - mb[k]
				diff2 += d * d
				ref2 += ma[k] * ma[k]
				logSum += math.Abs(math.Log(ma[k]+logMagEps) - math.Log(mb[k]+logMagEps))
				count++
			}
		}
		s := MRSTFTSize{FFT: size, LogMagnitude: logSum / float64(count)}
		switch {
		case ref2 > 0:
			s.SpectralConvergence = math.Sqrt(diff2 / ref2)
		case diff2 > 0:
			s.SpectralConvergence = 1
		}
		r.Sizes = append(r.Sizes, s)
		r.SpectralConvergence += s.SpectralConvergence
		r.LogMagnitude += s.LogMagnitude
	}
	if k := float64(len(r.Sizes)); k > 0 {
		r.SpectralConvergence /= k
		r.LogMagnitude /= k
		r.Total = r.SpectralConvergence + r.LogMagnitude
	}
	return r
}

// CrestFactorDB returns the peak to RMS ratio of x in decibels.
func CrestFactorDB(x []float64) float64 {
	var peak, sum float64
	for _, v := range x {
		peak = max(peak, math.Abs(v))
		sum += v * v
	}
	if sum == 0 {
		return 0
	}
	return 20 * math.Log10(peak/math.Sqrt(sum/float64(len(x))))
}

// SpectralFlux returns the mean Euclidean distance between successive
// unit-norm magnitude spectra (FFT 2048, hop 512). It is 0 for a steady
// spectrum and at most the square root of 2.
func SpectralFlux(x []float64) float64 {
	if len(x) < fluxFFT+fluxHop {
		return 0
	}
	sp := newSpectrum(fluxFFT)
	cur := make([]float64, fluxFFT/2+1)
	prev := make([]float64, fluxFFT/2+1)
	var sum float64
	frames := 0
	for start := 0; start+fluxFFT <= len(x); start += fluxHop {
		sp.magnitude(x[start:start+fluxFFT], cur)
		unitNorm(cur)
		if start > 0 {
			var d2 float64
			for k := range cur {
				d := cur[k] - prev[k]
				d2 += d * d
			}
			sum += math.Sqrt(d2)
			frames++
		}
		cur, prev = prev, cur
	}
	return sum / float64(frames)
}

func unitNorm(x []float64) {
	var s float64
	for _, v := range x {
		s += v * v
	}
	if s == 0 {
		return
	}
	s = 1 / math.Sqrt(s)
	for i := range x {
		x[i] *= s
	}
}
