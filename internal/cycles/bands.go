package cycles

import (
	"math"
)

// BandCount is the number of residual bands: octaves down from the Nyquist
// frequency of the input rate, the lowest band a low pass. The lowest edge
// is 10 to 39 Hz for input rates of 5 to 20 kHz, near the 10 Hz DC filter
// of the synthesiser.
const BandCount = 9

// BandEdges returns the BandCount+1 band edges in Hz for the input rate:
// 0, then the Nyquist frequency over 2^(BandCount-1) to 2, then the
// Nyquist frequency.
func BandEdges(rate float64) []float64 {
	ny := rate / 2
	e := make([]float64, BandCount+1)
	for i := 1; i < BandCount; i++ {
		e[i] = ny / math.Exp2(float64(BandCount-i))
	}
	e[BandCount] = ny
	return e
}

// biquad is a second-order section in direct form I, in float64.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (q *biquad) process(x float64) float64 {
	y := q.b0*x + q.b1*q.x1 + q.b2*q.x2 - q.a1*q.y1 - q.a2*q.y2
	q.x2, q.x1 = q.x1, x
	q.y2, q.y1 = q.y1, y
	return y
}

// block runs process over x in place, with the state in local variables.
// Each output is the output of process.
func (q *biquad) block(x []float64) {
	b0, b1, b2, a1, a2 := q.b0, q.b1, q.b2, q.a1, q.a2
	x1, x2, y1, y2 := q.x1, q.x2, q.y1, q.y2
	for i, v := range x {
		y := b0*v + b1*x1 + b2*x2 - a1*y1 - a2*y2
		x2, x1 = x1, v
		y2, y1 = y1, y
		x[i] = y
	}
	q.x1, q.x2, q.y1, q.y2 = x1, x2, y1, y2
}

// response returns the squared magnitude of the section at the normalised
// angular frequency w.
func (q *biquad) response(w float64) float64 {
	z1 := complex(math.Cos(w), -math.Sin(w))
	z2 := z1 * z1
	num := complex(q.b0, 0) + complex(q.b1, 0)*z1 + complex(q.b2, 0)*z2
	den := 1 + complex(q.a1, 0)*z1 + complex(q.a2, 0)*z2
	h := num / den
	return real(h)*real(h) + imag(h)*imag(h)
}

// butterworthQ are the section Q values of a fourth-order Butterworth
// filter: 1 / (2 cos(pi/8)) and 1 / (2 cos(3 pi/8)).
var butterworthQ = []float64{0.5411961001461969, 1.3065629648763766}

// section returns a second-order low pass, or high pass when high is
// true, at cutoff Hz with the given Q (the bilinear designs of the Audio
// EQ Cookbook).
func section(cutoff, rate, q float64, high bool) biquad {
	w0 := 2 * math.Pi * cutoff / rate
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * q)
	a0 := 1 + alpha
	s := biquad{a1: -2 * cw / a0, a2: (1 - alpha) / a0}
	if high {
		s.b0 = (1 + cw) / 2 / a0
		s.b1 = -(1 + cw) / a0
	} else {
		s.b0 = (1 - cw) / 2 / a0
		s.b1 = (1 - cw) / a0
	}
	s.b2 = s.b0
	return s
}

// band is one residual band filter: a fourth-order Butterworth high pass
// at the lower edge and a fourth-order Butterworth low pass at the upper
// edge. The lowest band has no high pass and the highest no low pass.
type band struct {
	sections []biquad
}

func newBand(lo, hi, rate float64) band {
	var b band
	for _, q := range butterworthQ {
		if lo > 0 {
			b.sections = append(b.sections, section(lo, rate, q, true))
		}
		if hi < rate/2 {
			b.sections = append(b.sections, section(hi, rate, q, false))
		}
	}
	return b
}

func (b *band) process(x float64) float64 {
	for i := range b.sections {
		x = b.sections[i].process(x)
	}
	return x
}

// block runs process over x in place. Each output is the output of
// process. In a cascade, the input history of a section is the output
// history of the section before it, so that a cascade of two or four
// sections runs in one loop with the states in local variables and the
// coefficients read from memory. The sections of one sample depend on each
// other, but each recursion depends only on its own section, so that the
// sections of successive samples overlap in time.
func (b *band) block(x []float64) {
	s := b.sections
	switch len(s) {
	case 2:
		p, q := &s[0], &s[1]
		x1, x2 := p.x1, p.x2
		py1, py2, qy1, qy2 := p.y1, p.y2, q.y1, q.y2
		for i, v := range x {
			y := p.b0*v + p.b1*x1 + p.b2*x2 - p.a1*py1 - p.a2*py2
			x2, x1 = x1, v
			z := q.b0*y + q.b1*py1 + q.b2*py2 - q.a1*qy1 - q.a2*qy2
			py2, py1 = py1, y
			qy2, qy1 = qy1, z
			x[i] = z
		}
		p.x1, p.x2, p.y1, p.y2 = x1, x2, py1, py2
		q.x1, q.x2, q.y1, q.y2 = py1, py2, qy1, qy2
	case 4:
		p, q, r, t := &s[0], &s[1], &s[2], &s[3]
		x1, x2 := p.x1, p.x2
		py1, py2, qy1, qy2 := p.y1, p.y2, q.y1, q.y2
		ry1, ry2, ty1, ty2 := r.y1, r.y2, t.y1, t.y2
		for i, v := range x {
			y := p.b0*v + p.b1*x1 + p.b2*x2 - p.a1*py1 - p.a2*py2
			x2, x1 = x1, v
			z := q.b0*y + q.b1*py1 + q.b2*py2 - q.a1*qy1 - q.a2*qy2
			py2, py1 = py1, y
			u := r.b0*z + r.b1*qy1 + r.b2*qy2 - r.a1*ry1 - r.a2*ry2
			qy2, qy1 = qy1, z
			w := t.b0*u + t.b1*ry1 + t.b2*ry2 - t.a1*ty1 - t.a2*ty2
			ry2, ry1 = ry1, u
			ty2, ty1 = ty1, w
			x[i] = w
		}
		p.x1, p.x2, p.y1, p.y2 = x1, x2, py1, py2
		q.x1, q.x2, q.y1, q.y2 = py1, py2, qy1, qy2
		r.x1, r.x2, r.y1, r.y2 = qy1, qy2, ry1, ry2
		t.x1, t.x2, t.y1, t.y2 = ry1, ry2, ty1, ty2
	default:
		for i := range s {
			s[i].block(x)
		}
	}
}

func (b *band) response(w float64) float64 {
	g := 1.0
	for i := range b.sections {
		g *= b.sections[i].response(w)
	}
	return g
}

// bankFor returns the BandCount band filters at the input rate.
func bankFor(rate float64) []band {
	e := BandEdges(rate)
	out := make([]band, BandCount)
	for i := range out {
		out[i] = newBand(e[i], e[i+1], rate)
	}
	return out
}

// bankGains returns the power gain of each band filter for unit white
// noise, and the leakage matrix: leak[b][c] is the power that band filter
// b measures from the noise of band c, normalised to unit power. A noise
// made of band c noises with powers p[c] measures sum_c leak[b][c] p[c] in
// band b.
func bankGains(bank []band) (gain []float64, leak [][]float64) {
	const n = 8192
	gain = make([]float64, len(bank))
	leak = make([][]float64, len(bank))
	resp := make([][]float64, len(bank))
	for b := range bank {
		resp[b] = make([]float64, n)
		for i := range n {
			resp[b][i] = bank[b].response(math.Pi * (float64(i) + 0.5) / n)
			gain[b] += resp[b][i] / n
		}
	}
	for b := range bank {
		leak[b] = make([]float64, len(bank))
		for c := range bank {
			var s float64
			for i := range n {
				s += resp[b][i] * resp[c][i] / n
			}
			leak[b][c] = s / gain[c]
		}
	}
	return gain, leak
}

// solve returns x with a x = y for a small dense matrix, by Gaussian
// elimination with partial pivoting.
func solve(a [][]float64, y []float64) []float64 {
	n := len(y)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append(append([]float64(nil), a[i]...), y[i])
	}
	for c := range n {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		m[c], m[p] = m[p], m[c]
		for r := c + 1; r < n; r++ {
			f := m[r][c] / m[c][c]
			for k := c; k <= n; k++ {
				m[r][k] -= f * m[c][k]
			}
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := m[r][n]
		for k := r + 1; k < n; k++ {
			s -= m[r][k] * x[k]
		}
		x[r] = s / m[r][r]
	}
	return x
}

// unmix returns the band noise powers p whose measure through the leakage
// matrix comes closest to the measured powers m in relative terms: it
// minimises sum_b ((leak p)_b / m_b - 1)^2 by coordinate descent from the
// exact solution. No band falls below 0.03 (-15 dB) of m_b / leak[b][b], the
// power that band b measures without the unmixing, so that a band whose
// measure the neighbours explain does not leave a hole in the spectrum.
func unmix(leak [][]float64, m []float64) []float64 {
	n := len(m)
	floor := make([]float64, n)
	for b := range floor {
		floor[b] = 0.03 * m[b] / leak[b][b]
	}
	p := solve(leak, m)
	for i := range p {
		if !(p[i] > floor[i]) {
			p[i] = floor[i]
		}
	}
	for range 200 {
		for j := range n {
			// The weighted least squares optimum of p[j] with the others
			// fixed: sum_b w_b a_bj (sum_k a_bk p_k - m_b) = 0, w_b = 1/m_b^2.
			var num, den float64
			for b := range n {
				if m[b] <= 0 {
					continue
				}
				w := 1 / (m[b] * m[b])
				rest := m[b]
				for k := range n {
					if k != j {
						rest -= leak[b][k] * p[k]
					}
				}
				num += w * leak[b][j] * rest
				den += w * leak[b][j] * leak[b][j]
			}
			if den > 0 {
				p[j] = max(floor[j], num/den)
			}
		}
	}
	return p
}
