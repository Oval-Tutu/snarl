package cycles

import (
	"errors"
	"math"
	"math/cmplx"
)

// Spectra are the power spectral densities of a channel input over a
// hold, and of its residual after the table point of the hold, on the
// frame/2+1 bins of a Welch average (Hann frames, half overlap). The
// values are mean power per bin.
type Spectra struct {
	BinHz           float64
	Input, Residual []float64
}

// HoldSpectra measures the spectra of channel c over the hold, with the
// residual after the average cycle of p, the table point of the same hold.
func (r *Record) HoldSpectra(seg RecordSegment, opt MeasureOptions, p *Point, c, frame int) (Spectra, error) {
	if frame < 16 || frame&(frame-1) != 0 {
		return Spectra{}, errors.New("the frame must be a power of two of at least 16")
	}
	s, err := r.HoldSpan(seg, opt.Settle)
	if err != nil {
		return Spectra{}, err
	}
	d := int(math.Round(r.ChannelDelays()[c]))
	shift := 0.0
	if opt.CrankPhase {
		d, shift = 0, r.ChannelDelays()[c]*r.Timestep*p.MeasuredRPM/120
	}
	table := NewWaveTable(p.Wave[c])
	x := r.Inputs[c][s.I0:s.I1]
	smooth := r.smoothedPhase(s, d, float64(s.I1-s.I0)/float64(s.Cycles), opt)
	phase := smooth.slice(s.I0-d, s.I1-d)
	res := make([]float64, len(x))
	for k, v := range x {
		res[k] = v - table.At(phase[k]-shift)
	}
	out := Spectra{BinHz: r.InputSampleRate / float64(frame)}
	out.Input = welch(x, frame)
	out.Residual = welch(res, frame)
	if out.Input == nil {
		return Spectra{}, errors.New("the hold is shorter than one frame")
	}
	return out, nil
}

// HoldModulation returns, for each slot count, the share of the residual
// power of channel c over the hold that a modulation of the average cycle
// of p in that many equal parts of each cycle explains, and the RMS gain of
// the playback (fitSlotModulation).
func (r *Record) HoldModulation(seg RecordSegment, opt MeasureOptions, p *Point, c int, slots []int) (share, depth []float64, err error) {
	s, err := r.HoldSpan(seg, opt.Settle)
	if err != nil {
		return nil, nil, err
	}
	d := int(math.Round(r.ChannelDelays()[c]))
	x := r.Inputs[c][s.I0:s.I1]
	smooth := r.smoothedPhase(s, d, float64(s.I1-s.I0)/float64(s.Cycles), opt)
	phase := smooth.slice(s.I0-d, s.I1-d)
	for _, n := range slots {
		_, dep, _, sh := fitSlotModulation(x, phase, p.Wave[c], 0, n)
		share = append(share, sh)
		depth = append(depth, dep)
	}
	return share, depth, nil
}

// ModelSpectrum returns the power spectral density of the residual model
// of channel c of the table point p, the band noise of the playback, on the
// frame/2+1 bins of HoldSpectra at the input rate: per band the mean power
// of its envelope over the sectors, shaped by the band filter.
func ModelSpectrum(p *Point, c int, rate float64, frame int) []float64 {
	bank := bankFor(rate)
	gains, _ := bankGains(bank)
	out := make([]float64, frame/2+1)
	for b := range bank {
		var power float64
		for _, v := range p.Noise[c][b*Sectors : (b+1)*Sectors] {
			power += v * v
		}
		power /= Sectors
		for k := range out {
			out[k] += power * bank[b].response(2*math.Pi*float64(k)/float64(frame)) / gains[b]
		}
	}
	return out
}

// BandPowers returns the power of a spectrum on the bins of HoldSpectra
// at the input rate through each residual band filter, as the measure of
// the residual table sees it.
func BandPowers(psd []float64, rate float64) []float64 {
	bank := bankFor(rate)
	frame := 2 * (len(psd) - 1)
	out := make([]float64, len(bank))
	for b := range bank {
		for k, v := range psd {
			out[b] += v * bank[b].response(2*math.Pi*float64(k)/float64(frame))
		}
		out[b] /= float64(len(psd))
	}
	return out
}

// Welch returns the mean power spectrum of x over Hann frames with half
// overlap on frame/2+1 bins, normalised so that white noise of unit
// variance gives 1 per bin on average over the bins, or nil when x is
// shorter than one frame.
func Welch(x []float64, frame int) []float64 { return welch(x, frame) }

// welch returns the mean power spectrum of x over Hann frames with half
// overlap, normalised so that white noise of unit variance gives 1 per bin
// on average over the bins.
func welch(x []float64, frame int) []float64 {
	if len(x) < frame {
		return nil
	}
	w := make([]float64, frame)
	var wp float64
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(frame))
		wp += w[i] * w[i]
	}
	out := make([]float64, frame/2+1)
	buf := make([]complex128, frame)
	n := 0
	for start := 0; start+frame <= len(x); start += frame / 2 {
		for i := range buf {
			buf[i] = complex(x[start+i]*w[i], 0)
		}
		forwardFFT(buf)
		for k := range out {
			a := cmplx.Abs(buf[k])
			out[k] += a * a / wp
		}
		n++
	}
	for k := range out {
		out[k] /= float64(n)
	}
	return out
}

// forwardFFT computes sum_j x_j exp(-2 pi i j k / n) in place.
func forwardFFT(x []complex128) {
	for i := range x {
		x[i] = cmplx.Conj(x[i])
	}
	inverseFFT(x)
	for i := range x {
		x[i] = cmplx.Conj(x[i])
	}
}
