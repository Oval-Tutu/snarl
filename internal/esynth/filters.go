package esynth

import (
	"math"
	"math/big"
)

// enginePi is constants::pi of engine-sim (include/constants.h), which is
// shorter than math.Pi.
const enginePi = 3.14159265359

// enginePiFloat is static_cast<float>(constants::pi), the value that the
// float filters use.
var enginePiFloat = float64(float32(enginePi))

// float32Min is FLT_MIN, the smallest normal float.
const float32Min = 0x1p-126

// Precision selects the arithmetic of the port.
type Precision uint8

const (
	// Float64 computes in float64 throughout.
	Float64 Precision = iota
	// Float32 rounds to float32 after each operation that engine-sim
	// computes in float, and at each float variable that it stores, so
	// that the port reproduces the engine-sim output bit for bit.
	Float32
)

// r rounds x to float32 in Float32 precision. In engine-sim, x is the
// result of a float operation or a value stored as float.
func (p Precision) r(x float64) float64 {
	if p == Float32 {
		return float64(float32(x))
	}
	return x
}

// butterworthLowPass ports ButterworthLowPassFilter<T_Real>
// (include/butterworth_low_pass_filter.h), a fourth-order Butterworth low
// pass in direct form. pi is static_cast<T_Real>(constants::pi), and p
// rounds the T_Real operations.
type butterworthLowPass struct {
	y, x *ringBuffer[float64]
	a    [5]float64
	f4   float64
	pi   float64
	p    Precision
}

// newButterworthLowPass ports the ButterworthLowPassFilter constructor. Set
// single for ButterworthLowPassFilter<float>, which casts pi to float and
// computes in float in Float32 precision.
func newButterworthLowPass(single bool, p Precision) *butterworthLowPass {
	b := &butterworthLowPass{y: newRingBuffer[float64](4), x: newRingBuffer[float64](4), pi: enginePi}
	if single {
		b.pi = enginePiFloat
		b.p = p
	}
	for range 4 {
		b.y.write(0)
		b.x.write(0)
	}
	return b
}

// fastF ports ButterworthLowPassFilter::fast_f.
func (b *butterworthLowPass) fastF(sample float64) float64 {
	r := b.p.r
	yPrev := [4]float64{b.y.at(3), b.y.at(2), b.y.at(1), b.y.at(0)}
	xPrev := [4]float64{b.x.at(3), b.x.at(2), b.x.at(1), b.x.at(0)}

	sum := r(sample + r(4*xPrev[0]))
	sum = r(sum + r(6*xPrev[1]))
	sum = r(sum + r(4*xPrev[2]))
	sum = r(sum + xPrev[3])
	n := r(r(b.f4/b.a[0]) * sum)
	d := r(-b.a[1] * yPrev[0])
	d = r(d - r(b.a[2]*yPrev[1]))
	d = r(d - r(b.a[3]*yPrev[2]))
	d = r(d - r(b.a[4]*yPrev[3]))
	y := r(n + d)

	b.x.removeBeginning(1)
	b.x.write(sample)

	b.y.removeBeginning(1)
	b.y.write(y)

	return y
}

// setCutoffFrequency ports ButterworthLowPassFilter::setCutoffFrequency.
// The coefficient sums mix T_Real and double literals, so they run in
// double and round once when the coefficient is stored.
func (b *butterworthLowPass) setCutoffFrequency(fc, sampleRate float64) {
	r := b.p.r
	f := roundedTan(r(r(b.pi*fc)/sampleRate), b.p)
	f2 := r(f * f)
	f3 := r(f2 * f)
	f4 := r(f2 * f2)
	m := r(-2.0 * roundedCos(5.0*enginePi/8.0))
	n := r(-2.0 * roundedCos(7.0*enginePi/8.0))
	mPlusN := r(m + n)
	mTimesN := r(m * n)

	b.a[0] = r(1.0 + r(mPlusN*f) + (2.0+mTimesN)*f2 + r(mPlusN*f3) + f4)
	b.a[1] = r((-4.0 - 2.0*mPlusN*f + 2.0*mPlusN*f3 + 4.0*f4) / b.a[0])
	b.a[2] = r((6.0 - 2.0*(2.0+mTimesN)*f2 + 6.0*f4) / b.a[0])
	b.a[3] = r((-4.0 + 2.0*mPlusN*f - 2.0*mPlusN*f3 + 4.0*f4) / b.a[0])
	b.a[4] = r((1.0 - r(mPlusN*f) + (2.0+mTimesN)*f2 - r(mPlusN*f3) + f4) / b.a[0])

	b.f4 = f4
}

// lowPass ports LowPassFilter (include/low_pass_filter.h,
// src/low_pass_filter.cpp), a one-pole low pass in float.
type lowPass struct {
	dt float64
	y  float64
	rc float64
	p  Precision
}

// newLowPass ports the LowPassFilter constructor.
func newLowPass(p Precision) *lowPass {
	return &lowPass{dt: float64(float32(1) / float32(44100)), p: p}
}

// fastF ports LowPassFilter::fast_f.
func (l *lowPass) fastF(sample float64) float64 {
	r := l.p.r
	alpha := r(l.dt / r(l.rc+l.dt))
	l.y = r(r(alpha*sample) + r(r(1-alpha)*l.y))
	return l.y
}

// setCutoffFrequency ports LowPassFilter::setCutoffFrequency.
func (l *lowPass) setCutoffFrequency(f float64) {
	r := l.p.r
	l.rc = r(1.0 / r(r(f*2.0)*enginePiFloat))
}

// derivative ports DerivativeFilter (src/derivative_filter.cpp) in float.
type derivative struct {
	dt       float64
	previous float64
	p        Precision
}

// f ports DerivativeFilter::f.
func (d *derivative) f(sample float64) float64 {
	temp := d.previous
	d.previous = sample
	return d.p.r(d.p.r(sample-temp) / d.dt)
}

// convolution ports ConvolutionFilter (src/convolution_filter.cpp), a
// direct-form FIR filter in float over a circular shift register.
type convolution struct {
	shiftRegister   []float64
	shiftOffset     int
	impulseResponse []float64
	p               Precision
}

// initialize ports ConvolutionFilter::initialize.
func (c *convolution) initialize(samples int) {
	c.shiftOffset = 0
	c.shiftRegister = make([]float64, samples)
	c.impulseResponse = make([]float64, samples)
}

// f ports ConvolutionFilter::f.
func (c *convolution) f(sample float64) float64 {
	n := len(c.impulseResponse)
	c.shiftRegister[c.shiftOffset] = sample

	result := 0.0
	if c.p == Float32 {
		for i := range n - c.shiftOffset {
			result = float64(float32(result + float64(float32(c.impulseResponse[i]*c.shiftRegister[i+c.shiftOffset]))))
		}
		for i := n - c.shiftOffset; i < n; i++ {
			result = float64(float32(result + float64(float32(c.impulseResponse[i]*c.shiftRegister[i-(n-c.shiftOffset)]))))
		}
	} else {
		for i := range n - c.shiftOffset {
			result += c.impulseResponse[i] * c.shiftRegister[i+c.shiftOffset]
		}
		for i := n - c.shiftOffset; i < n; i++ {
			result += c.impulseResponse[i] * c.shiftRegister[i-(n-c.shiftOffset)]
		}
	}

	c.shiftOffset = (c.shiftOffset - 1 + n) % n
	return result
}

// leveling ports LevelingFilter (src/leveling_filter.cpp), a peak
// follower in float that scales the signal towards a target peak.
type leveling struct {
	peak        float64
	attenuation float64
	maxLevel    float64
	minLevel    float64
	target      float64
	p           Precision
}

// newLeveling ports the LevelingFilter constructor.
func newLeveling(p Precision) *leveling {
	return &leveling{peak: 30000.0, attenuation: 1.0, target: 30000.0, minLevel: 0.0, maxLevel: 1.0, p: p}
}

// f ports LevelingFilter::f.
func (l *leveling) f(sample float64) float64 {
	r := l.p.r
	l.peak = r(float64(float32(0.999)) * l.peak)
	if math.Abs(sample) > l.peak {
		l.peak = math.Abs(sample)
	}

	if l.peak == 0 {
		return 0
	}

	rawAttenuation := r(l.target / l.peak)

	attenuation := rawAttenuation
	if attenuation < l.minLevel {
		attenuation = l.minLevel
	} else if attenuation > l.maxLevel {
		attenuation = l.maxLevel
	}

	// 0.9 and 0.1 are double literals: the sum runs in double.
	l.attenuation = r(0.9*l.attenuation + 0.1*attenuation)

	return r(sample * l.attenuation)
}

// jitter ports JitterFilter (include/jitter_filter.h,
// src/jitter_filter.cpp): a short float history read at a random,
// low-passed fractional position.
type jitter struct {
	noiseFilter *butterworthLowPass
	jitterScale float64
	maxJitter   int
	offset      int
	history     []float64
	generator   minstdRand0
	p           Precision
}

// newJitter ports the JitterFilter constructor and
// JitterFilter::initialize.
func newJitter(maxJitter int, cutoffFrequency, audioFrequency float64, p Precision) *jitter {
	j := &jitter{
		noiseFilter: newButterworthLowPass(true, p),
		maxJitter:   maxJitter,
		history:     make([]float64, maxJitter),
		generator:   newMinstdRand0(),
		p:           p,
	}
	j.noiseFilter.setCutoffFrequency(cutoffFrequency, audioFrequency)
	return j
}

// fastF ports JitterFilter::fast_f with jitterScale 1, as
// Synthesizer::renderAudio(int) calls it.
func (j *jitter) fastF(sample float64) float64 {
	r := j.p.r
	j.history[j.offset] = sample
	j.offset++

	if j.offset >= j.maxJitter {
		j.offset = 0
	}

	top := float64(j.maxJitter - 1)
	draw := float64(j.generator.uniformFloat32(0, float32(j.maxJitter-1)))
	s := j.noiseFilter.fastF(r(r(draw*j.jitterScale) * 1.0))
	si0 := clamp(math.Floor(s), 0, top)
	si1 := clamp(math.Ceil(s), 0, top)

	sFrac := r(s - si0)

	i0 := int(si0) + j.offset
	i1 := int(si1) + j.offset

	if i0 >= j.maxJitter {
		i0 -= j.maxJitter
	}
	if i1 >= j.maxJitter {
		i1 -= j.maxJitter
	}
	v0 := j.history[i0]
	v1 := j.history[i1]

	return r(r(v1*sFrac) + r(v0*r(1-sFrac)))
}

// clamp ports clamp (include/utilities.h).
func clamp(x, x0, x1 float64) float64 {
	if x <= x0 {
		return x0
	} else if x >= x1 {
		return x1
	}
	return x
}

// roundedTan returns tan(x) rounded to the nearest float64, or to the
// nearest float32 in Float32 precision, as glibc tan and tanf return it for
// the filter arguments. math.Tan can be 1 ulp off, and 1 ulp in a
// coefficient of the 1900 Hz double filter changes the output in rare
// samples.
func roundedTan(x float64, p Precision) float64 {
	sin, cos := sinCos(x)
	t := new(big.Float).SetPrec(sinCosPrec).Quo(sin, cos)
	if p == Float32 {
		f, _ := t.Float32()
		return float64(f)
	}
	f, _ := t.Float64()
	return f
}

// roundedCos returns cos(x) rounded to the nearest float64.
func roundedCos(x float64) float64 {
	_, cos := sinCos(x)
	c, _ := cos.Float64()
	return c
}

const sinCosPrec = 256

// sinCos sums the Taylor series of sin(x) and cos(x) for |x| below 4 to
// 256 bits.
func sinCos(x float64) (sin, cos *big.Float) {
	newF := func() *big.Float { return new(big.Float).SetPrec(sinCosPrec) }
	bx := newF().SetFloat64(x)
	x2 := newF().Mul(bx, bx)
	sin, cos = newF().Set(bx), newF().SetInt64(1)
	termSin, termCos := newF().Set(bx), newF().SetInt64(1)
	for k := int64(1); k <= 60; k++ {
		termSin.Mul(termSin, x2).Neg(termSin).Quo(termSin, newF().SetInt64(2*k*(2*k+1)))
		termCos.Mul(termCos, x2).Neg(termCos).Quo(termCos, newF().SetInt64((2*k-1)*2*k))
		sin.Add(sin, termSin)
		cos.Add(cos, termCos)
	}
	return sin, cos
}

// float32Subnormal reports whether fpclassify(float(v)) is FP_SUBNORMAL.
func float32Subnormal(v float64) bool {
	return v != 0 && math.Abs(v) < float32Min
}
