package dsp

import (
	"fmt"
	"math"
)

// Resampler converts a float32 stream from one sample rate to another with
// a polyphase windowed-sinc filter: a Kaiser-windowed sinc low pass with
// taps coefficients per phase and one phase per output position on the
// grid of the two rates. Output sample j is the input interpolated at time
// j * from / to in input samples, so the output keeps the time of the
// input. The filter reads taps/2 input samples ahead, which is the delay of
// the stream. Push and Pull do not allocate.
type Resampler struct {
	up, down, taps int
	// coef holds the coefficients of each phase, phase after phase.
	coef []float32
	// buf holds the input from taps/2-1 samples before pos.
	buf []float32
	n   int
	// pos is the index in buf of the input sample at or before the time
	// of the next output, and frac/up the fraction from it.
	pos, frac int
}

// ResamplerConfig is the design of a Resampler.
type ResamplerConfig struct {
	// From and To are the input and output rates in hertz.
	From, To int
	// Taps is the number of coefficients per phase, even.
	Taps int
	// Cutoff is the -6 dB frequency of the low pass in hertz, at most half
	// the lower rate.
	Cutoff float64
	// Beta is the Kaiser window parameter.
	Beta float64
	// MaxPush is the most input samples that one Push takes.
	MaxPush int
}

// NewResampler designs a resampler.
func NewResampler(c ResamplerConfig) (*Resampler, error) {
	if c.From <= 0 || c.To <= 0 || c.Taps < 2 || c.Taps%2 != 0 || c.MaxPush < 1 {
		return nil, fmt.Errorf("dsp: resampler rates %d and %d, %d taps and %d push: want positive rates, even taps and a positive push", c.From, c.To, c.Taps, c.MaxPush)
	}
	if c.Cutoff <= 0 || c.Cutoff > 0.5*float64(min(c.From, c.To)) {
		return nil, fmt.Errorf("dsp: resampler cutoff %g Hz is not in (0, %g]", c.Cutoff, 0.5*float64(min(c.From, c.To)))
	}
	g := gcd(c.From, c.To)
	r := &Resampler{up: c.To / g, down: c.From / g, taps: c.Taps}
	r.coef = make([]float32, r.up*r.taps)
	half := r.taps / 2
	fc := c.Cutoff / float64(c.From)
	i0Beta := besselI0(c.Beta)
	for p := range r.up {
		row := r.coef[p*r.taps : (p+1)*r.taps]
		sum := 0.0
		h := make([]float64, r.taps)
		for k := range h {
			// The distance of tap k from the output time.
			d := float64(p)/float64(r.up) + float64(half-1-k)
			u := d / float64(half)
			w := 0.0
			if u > -1 && u < 1 {
				w = besselI0(c.Beta*math.Sqrt(1-u*u)) / i0Beta
			}
			h[k] = 2 * fc * sinc(2*fc*d) * w
			sum += h[k]
		}
		for k := range row {
			row[k] = float32(h[k] / sum)
		}
	}
	r.buf = make([]float32, r.taps+c.MaxPush)
	r.Reset()
	return r, nil
}

// Reset clears the input history. The next output is at the time of the
// next input sample.
func (r *Resampler) Reset() {
	clear(r.buf)
	r.n = r.taps/2 - 1
	r.pos = r.n
	r.frac = 0
}

// Delay returns the input samples that the resampler must hold beyond the
// time of an output before it can compute it.
func (r *Resampler) Delay() int { return r.taps / 2 }

// Push appends input samples and returns how many it took. It takes all
// of at most MaxPush samples when Pull ran out of input since the last
// Push.
func (r *Resampler) Push(x []float32) int {
	if shift := r.pos - (r.taps/2 - 1); shift > 0 {
		copy(r.buf, r.buf[shift:r.n])
		r.n -= shift
		r.pos -= shift
	}
	n := copy(r.buf[r.n:], x)
	r.n += n
	return n
}

// Pull writes as many output samples to y as the input allows and returns
// how many it wrote.
func (r *Resampler) Pull(y []float32) int {
	half := r.taps / 2
	for j := range y {
		if r.pos+half >= r.n {
			return j
		}
		row := r.coef[r.frac*r.taps : (r.frac+1)*r.taps]
		x := r.buf[r.pos-half+1 : r.pos+half+1]
		x = x[:len(row)]
		// Four sums run in parallel.
		var a0, a1, a2, a3 float32
		k := 0
		for ; k+4 <= len(row); k += 4 {
			a0 += row[k] * x[k]
			a1 += row[k+1] * x[k+1]
			a2 += row[k+2] * x[k+2]
			a3 += row[k+3] * x[k+3]
		}
		for ; k < len(row); k++ {
			a0 += row[k] * x[k]
		}
		y[j] = (a0 + a1) + (a2 + a3)
		r.frac += r.down
		for r.frac >= r.up {
			r.frac -= r.up
			r.pos++
		}
	}
	return len(y)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// besselI0 returns the modified Bessel function of the first kind of order
// 0 by its power series.
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 200; k++ {
		term *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		sum += term
		if term < 1e-17*sum {
			break
		}
	}
	return sum
}
