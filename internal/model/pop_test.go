package model

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// TestPopShape checks one isolated pop against the measured shape: most
// energy from 120 to 500 Hz, a 2 to 8 kHz crack of a few per cent, and a
// decay to -20 dB in 15 to 60 ms.
func TestPopShape(t *testing.T) {
	s := popSpec()
	y, err := New(&s, rate)
	if err != nil {
		t.Fatal(err)
	}
	y.voiceGain = 0
	y.pops.heat = 1
	buf := make([]float32, 64)
	c := Control{Running: true}
	for range 200 {
		y.Render(buf, c)
	}
	y.pops.fire(1, kindPop, srcOwn, 1/y.pops.kindGain[0][0][kindPop], 1, 1)
	x := make([]float32, rate*3/10)
	y.Render(x, c)
	var e [4]float64
	edges := []float64{20, 120, 500, 2000, 8000}
	for f := 20.0; f < 8000; f += 5 {
		var re, im float64
		for i, v := range x {
			w := 2 * math.Pi * f * float64(i) / rate
			re += float64(v) * math.Cos(w)
			im += float64(v) * math.Sin(w)
		}
		for b := range 4 {
			if f >= edges[b] && f < edges[b+1] {
				e[b] += re*re + im*im
			}
		}
	}
	tot := e[0] + e[1] + e[2] + e[3]
	// Envelope: 1 ms RMS, peak and time to -20 dB.
	var env []float64
	for i := 0; i+48 <= len(x); i += 48 {
		var s float64
		for _, v := range x[i : i+48] {
			s += float64(v * v)
		}
		env = append(env, math.Sqrt(s/48))
	}
	pk, pi := 0.0, 0
	for i, v := range env {
		if v > pk {
			pk, pi = v, i
		}
	}
	d := 0
	for i := len(env) - 1; i > pi; i-- {
		if env[i] > pk/10 {
			d = i - pi
			break
		}
	}
	body, crack := 100*e[1]/tot, 100*e[3]/tot
	if body < 45 || crack < 0.3 || crack > 3 || d < 15 || d > 60 {
		t.Errorf("120 to 500 Hz %.0f%%, 2 to 8 kHz %.1f%%, -20 dB after %d ms", body, crack, d)
	}
}

// BenchmarkPops renders a four-cylinder engine in a normal state with no
// pops and in a dense pop state, where a request every 60 ms keeps every
// pop slot busy.
func BenchmarkPops(b *testing.B) {
	for _, dense := range []bool{false, true} {
		b.Run(fmt.Sprintf("dense %t", dense), func(b *testing.B) {
			s := limiterSpec("fuel")
			s.Pops.Density = 1
			y, err := New(&s, rate)
			if err != nil {
				b.Fatal(err)
			}
			buf := make([]float32, 256)
			c := Control{RPM: 4000, Throttle: 0.7, Load: 0.7, Running: true, Gear: 3}
			y.Render(buf, c)
			if dense {
				c = Control{RPM: 6000, Running: true, Overrun: true, Gear: 3, PopStrength: 1}
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c.PopRequests = 0
				if dense && i%11 == 0 {
					c.PopRequests = 1
				}
				i++
				y.Render(buf, c)
			}
		})
	}
}

// popSpec is a one-cylinder engine with one path for isolated pops.
func popSpec() Spec {
	s := DefaultSpec()
	s.Name = "p"
	s.Pulse.Table = "i4-vtec"
	s.IdleRPM, s.RedlineRPM = 800, 6800
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, Bank: 0, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, Gain: 1}}
	setPopBank(&s)
	s.Pops.Density = 0.4
	return s
}

// setPopBank gives s the pop bank that tools/fit --pop fitted to the
// EngineLab pop stages of the test paths at a73b874, from
// testdata/legacy/model-test.toml.
func setPopBank(s *Spec) {
	s.Exhaust.PopFormantGainDB, s.Exhaust.PopDelayMS = -10.88, 0.708
	s.PopFormants = []FormantSpec{
		{Kind: "peak", Hz: 704.3, Q: 1.618, GainDB: 24.48},
		{Kind: "peak", Hz: 753.3, Q: 0.3, GainDB: -23.39},
		{Kind: "peak", Hz: 1396.1, Q: 5.356, GainDB: 12.99},
		{Kind: "peak", Hz: 2099.0, Q: 9.771, GainDB: 8.94},
		{Kind: "lowpass", Hz: 3368.0, Q: 1.186},
		{Kind: "peak", Hz: 6688.2, Q: 0.516, GainDB: 19.52},
		{Kind: "peak", Hz: 17413.6, Q: 0.717, GainDB: -11.76},
	}
}

// renderPop renders one isolated pop of a kind for 0.8 s. draws advances
// the noise first, so that each value gives another variation.
func renderPop(t testing.TB, s *Spec, kind, draws int) []float32 {
	t.Helper()
	y, err := New(s, rate)
	if err != nil {
		t.Fatal(err)
	}
	y.voiceGain = 0
	y.pops.heat = 1
	buf := make([]float32, 64)
	c := Control{Running: true}
	for range 200 {
		y.Render(buf, c)
	}
	for range draws {
		y.pops.noise.Next()
	}
	// The scale gives the pop kind a unit gain, and the other kinds their
	// gains over it.
	y.pops.fire(1, kind, srcOwn, 1/y.pops.kindGain[0][0][kindPop], 1, 1)
	x := make([]float32, rate*8/10)
	y.Render(x, c)
	return x
}

// popMeasure is the form of one pop, measured as the crackle pack was
// measured: a 1 ms RMS envelope at a 0.5 ms hop, smoothed over 3 ms.
type popMeasure struct {
	durMS    float64    // Time above the envelope peak minus 30 dB.
	decayMS  float64    // Time from the envelope peak to the peak minus 20 dB.
	centroid float64    // Spectral centroid from 30 Hz to 8 kHz.
	bands    [4]float64 // Shares of 30-120, 120-500, 500-2000 and 2000-8000 Hz.
	peakDB   float64    // Sample peak in dBFS.
}

func measurePop(x []float32) popMeasure {
	const hop, win = rate / 2000, rate / 1000
	var env []float64
	for i := 0; i+win <= len(x); i += hop {
		var s float64
		for _, v := range x[i : i+win] {
			s += float64(v * v)
		}
		env = append(env, s/win)
	}
	sm := make([]float64, len(env))
	for i := range env {
		var s float64
		for k := max(0, i-3); k < min(len(env), i+3); k++ {
			s += env[k]
		}
		sm[i] = s / 6
	}
	pi := 0
	for i, v := range sm {
		if v > sm[pi] {
			pi = i
		}
	}
	var m popMeasure
	first, last := -1, 0
	for i, v := range sm {
		if v > sm[pi]*1e-3 {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	m.durMS = float64(last-first) / 2
	d := pi
	for d < len(sm)-1 && sm[d] > sm[pi]*1e-2 {
		d++
	}
	m.decayMS = float64(d-pi) / 2
	// Band shares over the span above -30 dB, with a Hann window.
	a, b := first*hop, min(len(x), last*hop+win)
	n := 1
	for n < max(b-a, 8192) {
		n <<= 1
	}
	z := make([]complex128, n)
	for i := a; i < b; i++ {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i-a)/float64(b-a))
		z[i-a] = complex(float64(x[i])*w, 0)
	}
	analysis.FFT(z)
	edges := []float64{30, 120, 500, 2000, 8000}
	var tot, cen float64
	for k := 1; k < n/2; k++ {
		f := float64(k) * rate / float64(n)
		if f < 30 || f >= 8000 {
			continue
		}
		p := real(z[k])*real(z[k]) + imag(z[k])*imag(z[k])
		tot += p
		cen += f * p
		for j := range 4 {
			if f >= edges[j] && f < edges[j+1] {
				m.bands[j] += p
			}
		}
	}
	for j := range 4 {
		m.bands[j] /= tot
	}
	m.centroid = cen / tot
	var pk float64
	for _, v := range x {
		pk = math.Max(pk, math.Abs(float64(v)))
	}
	m.peakDB = 20 * math.Log10(pk)
	return m
}

// popRange is a measured range from the 10th to the 90th percentile of a
// cluster in the crackle pack (docs/research/crackles.md).
type popRange struct{ lo, hi float64 }

// TestPopKinds renders 12 variations of each pop kind and
// checks the median of each measure against the cluster of the crackle
// pack. Times and the centroid may miss the range by 20 per cent. A band
// share may miss it by 8 percentage points or by a factor of 3, whichever
// is wider, for the expansion chamber and the exhaust path. The pop keeps its
// tuned body: its 30 to 120 Hz share is below the cluster, so its band
// limits are those of TestPopShape. The level between the kinds comes
// from their levels over the engine, so the peak of the burble is within
// 4 dB of the difference of the pop and burble targets below the pop.
func TestPopKinds(t *testing.T) {
	s := popSpec()
	none := popRange{0, 100}
	want := [numKinds]struct {
		name            string
		dur, decay, cen popRange
		bands           [4]popRange
	}{
		kindPop:     {"pop", popRange{33, 118}, popRange{9, 35}, popRange{154, 353}, [4]popRange{none, {45, 100}, none, {0.3, 3}}},
		kindBoom:    {"boom", popRange{104, 291}, popRange{16, 91}, popRange{62, 175}, [4]popRange{{78, 97}, {1.8, 18.6}, {0.4, 3.5}, {0.1, 1.5}}},
		kindCrackle: {"crackle", popRange{165, 684}, popRange{23, 139}, popRange{167, 479}, [4]popRange{{62, 87}, {7, 26}, {2.4, 9}, {1.4, 7.2}}},
		kindBurble:  {"burble", popRange{38, 361}, popRange{11, 74}, popRange{41, 209}, [4]popRange{{15, 99.8}, {0.2, 83}, {0, 1.8}, {0, 0.05}}},
	}
	in := func(v float64, r popRange, share bool) bool {
		if !share {
			return v >= 0.8*r.lo && v <= 1.2*r.hi
		}
		return v >= min(r.lo-8, r.lo/3) && v <= max(r.hi+8, 3*r.hi)
	}
	var pop float64
	for k := range numKinds {
		w := want[k]
		var dur, dec, cen, pk []float64
		var bands [4][]float64
		for v := range 12 {
			m := measurePop(renderPop(t, &s, k, v*7))
			dur = append(dur, m.durMS)
			dec = append(dec, m.decayMS)
			cen = append(cen, m.centroid)
			pk = append(pk, m.peakDB)
			for b := range 4 {
				bands[b] = append(bands[b], 100*m.bands[b])
			}
		}
		med := func(x []float64) float64 {
			slices.Sort(x)
			return (x[5] + x[6]) / 2
		}
		md, mdc, mc, mp := med(dur), med(dec), med(cen), med(pk)
		var mb [4]float64
		ok := in(md, w.dur, false) && in(mdc, w.decay, false) && in(mc, w.cen, false)
		for b := range 4 {
			mb[b] = med(bands[b])
			ok = ok && in(mb[b], w.bands[b], true)
		}
		if k == kindPop {
			pop = mp
		}
		if gap := overDB[kindPop] - overDB[kindBurble]; k == kindBurble && math.Abs(pop-mp-gap) > 4 {
			ok = false
		}
		msg := fmt.Sprintf("%s: %.0f ms, decay %.0f ms, centroid %.0f Hz, bands %.1f/%.1f/%.2f/%.3f%%, peak %.1f dBFS",
			w.name, md, mdc, mc, mb[0], mb[1], mb[2], mb[3], mp)
		if !ok {
			t.Error(msg)
		} else {
			t.Log(msg)
		}
	}
}

// TestCrackleClicks renders 48 variations of the crackle and checks its
// clicks against the clicks of the crackle trains in the muffler crackle
// pack (docs/research/crackles.md section 11), with the same measure. The
// pack values are the medians over its files. The attack must be at most
// 1.25 times the pack attack, because a click starts at once. The decays,
// the centroid and the gap quantiles may miss the pack by 30 per cent, the
// crest factor by 1.5 dB, and the gap spread and local variation by 0.15.
func TestCrackleClicks(t *testing.T) {
	s := popSpec()
	var x []float64
	for v := range 48 {
		for _, f := range renderPop(t, &s, kindCrackle, v*7) {
			x = append(x, float64(f))
		}
	}
	st := measureClicks(x, rate)
	near := func(got, want, tol float64) bool { return math.Abs(got-want) <= tol*want }
	ok := st.attackMS <= 1.25*1.20 && near(st.decay10MS, 3.36, 0.3) && near(st.decayMS, 10.25, 0.3) &&
		math.Abs(st.crestDB-11.8) <= 1.5 && near(st.centroid, 2523, 0.3) &&
		near(st.gapP10, 5.0, 0.3) && near(st.gapP50, 9.8, 0.3) && near(st.gapP90, 24.5, 0.3) &&
		math.Abs(st.gapCV-0.74) <= 0.15 && math.Abs(st.lv-0.38) <= 0.15
	msg := fmt.Sprintf("%d clicks: attack %.2f ms, decay to -10 dB %.2f ms, to -20 dB %.2f ms, crest %.1f dB, centroid %.0f Hz, gaps %.1f/%.1f/%.1f ms, spread %.2f, local variation %.2f",
		st.n, st.attackMS, st.decay10MS, st.decayMS, st.crestDB, st.centroid, st.gapP10, st.gapP50, st.gapP90, st.gapCV, st.lv)
	if !ok {
		t.Error(msg)
	} else {
		t.Log(msg)
	}
}

// TestCalibrationRate checks that the kind gains put the pops of the
// calibration draws, rendered at 48 kHz through the output stage as the
// engine renders them, at their targets within 0.05 dB. The pop bank has a
// 15 kHz peak of +40 dB and no low pass, so that a calibration at half the
// rate, or through a low pass that the engine skips, misses the target of
// the pop by about 0.1 dB.
func TestCalibrationRate(t *testing.T) {
	s := popSpec()
	s.PopFormants = []FormantSpec{
		{Kind: "peak", Hz: 704.3, Q: 1.618, GainDB: 24.48},
		{Kind: "peak", Hz: 15000, Q: 1, GainDB: 40},
	}
	g := freshGains(&s)
	n := int(calSec * rate)
	for k := range numKinds {
		var lg float64
		for d := range calDraws {
			_, crack, out := PopSignals(&s, 0, k, d, rate, n)
			o := newOutput(0, 0, rate)
			x := make([]float32, n)
			for j := range n {
				v := o.dc.Process(out[j] + crack[j])
				if o.useLP {
					v = o.lp.Process(v)
				}
				x[j] = v
			}
			lg += math.Log10(spanRMS(x, int(0.01*rate)))
		}
		got := 20 * (math.Log10(float64(g[0][0][k])) + lg/calDraws)
		if want := overDB[k] + calTrimDB; math.Abs(got-want) > 0.05 {
			t.Errorf("kind %d: %.2f dB at 48 kHz, want %.2f dB", k, got, want)
		} else {
			t.Logf("kind %d: %.2f dB at 48 kHz, want %.2f dB", k, got, want)
		}
	}
}

// TestCrackDecay checks a crack that decays slower than the body: crack_ms
// 100 and body_ms 30. The crack decays smoothly until it is 120 dB down: no
// 5 ms window falls more than 6 dB below the window before it. A new pop in
// the slot starts with no crack envelope from the pop before.
func TestCrackDecay(t *testing.T) {
	s := popSpec()
	s.Pops.CrackMS, s.Pops.BodyMS = 100, 30
	p := newPops(&s, rate, chunk/float64(rate))
	e := p.fire(1, kindPop, srcOwn, 1, 1, 1)
	const win = rate / 200
	var prev, top float64
	for i := 0; e.left >= win; i++ {
		var en float64
		for range win {
			c := float64(p.tick())
			en += c * c
		}
		top = max(top, en)
		if i > 0 && prev >= 1e-12*top && en < prev/4 {
			t.Fatalf("at %d ms, the crack falls by more than 6 dB from %.1f dB below its peak",
				(i+1)*5, 10*math.Log10(top/prev))
		}
		prev = en
	}
	for e.left > 0 {
		p.tick()
	}
	if q := p.fire(1, kindPop, srcOwn, 1, 1, 1); q != e || q.crackEnv != 0 {
		t.Errorf("the next pop in the slot starts with a crack envelope of %g", q.crackEnv)
	}
}

// clickStats are the medians of the clicks in a signal, measured on the
// signal above 1 kHz.
type clickStats struct {
	n                 int
	attackMS, decayMS float64 // 10 to 90 per cent rise, and peak to -20 dB.
	censored          float64 // Share of clicks whose decay the next click cuts.
	crestDB           float64
	centroid          float64 // 1 to 16 kHz.
	gaps              []float64
	gapP10, gapP50    float64
	gapP90, gapCV     float64
	burst             float64 // (sd - mean) / (sd + mean) of the intervals.
	lv                float64 // Local variation of the intervals: 0 regular, 1 random.
	decay10MS         float64 // Peak to -10 dB.
	censored10        float64
}

// hp4 is a fourth-order Butterworth high pass in float64.
type hp4 struct {
	b [2][5]float64
	z [2][2]float64
}

// newHP returns the high pass at f hertz.
func newHP(f, rate float64) hp4 {
	var h hp4
	for i, q := range []float64{0.5412, 1.3066} {
		w := 2 * math.Pi * f / rate
		c, a := math.Cos(w), math.Sin(w)/(2*q)
		a0 := 1 + a
		h.b[i] = [5]float64{(1 + c) / 2 / a0, -(1 + c) / a0, (1 + c) / 2 / a0, -2 * c / a0, (1 - a) / a0}
	}
	return h
}

func (h *hp4) process(x float64) float64 {
	for i := range 2 {
		b, z := &h.b[i], &h.z[i]
		y := b[0]*x + z[0]
		z[0] = b[1]*x - b[3]*y + z[1]
		z[1] = b[2]*x - b[4]*y
		x = y
	}
	return x
}

// pct returns the p quantile of x.
func pct(x []float64, p float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := slices.Clone(x)
	slices.Sort(s)
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

// measureClicks finds the clicks of x in the signal above 1 kHz. A click
// is a peak of a centred 1 ms RMS envelope that is the highest within
// 3 ms, within 30 dB of the highest peak, and 6 dB above the lowest
// envelope since the click before. The attack is the 10 to 90 per cent
// rise of a 0.25 ms RMS envelope from that lowest value to the peak, and
// the decay the time from the peak of the 1 ms envelope to -20 dB.
func measureClicks(x []float64, rate float64) clickStats {
	h := newHP(1000, rate)
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = h.process(v)
	}
	rmsEnv := func(w int) []float64 {
		env := make([]float64, len(y))
		var s float64
		for i := -w / 2; i < len(y); i++ {
			if j := i + w/2; j < len(y) {
				s += y[j] * y[j]
			}
			if j := i - w/2 - 1; j >= 0 {
				s -= y[j] * y[j]
			}
			if i >= 0 {
				env[i] = math.Sqrt(max(s, 0) / float64(w))
			}
		}
		return env
	}
	envF := rmsEnv(max(1, int(math.Round(0.00025*rate))))
	env := rmsEnv(int(math.Round(0.001 * rate)))
	top := slices.Max(env)
	ms := int(0.001 * rate)
	var peaks []int
	low := math.Inf(1)
	for i := range env {
		low = min(low, env[i])
		if env[i] < top*0.0316 || env[i] != slices.Max(env[max(0, i-3*ms):min(len(env), i+3*ms+1)]) {
			continue
		}
		if len(peaks) > 0 && low > env[i]*0.5 {
			continue
		}
		peaks = append(peaks, i)
		low = env[i]
	}
	var st clickStats
	var last, lvSum float64
	lvN := 0
	var att, dec, crest, cen []float64
	cut, cut10 := 0, 0
	var dec10 []float64
	for k, p := range peaks {
		pk := env[p]
		prev := 0
		if k > 0 {
			prev = peaks[k-1]
		}
		q := p
		for i := max(prev, p-ms); i < min(len(envF), p+ms); i++ {
			if envF[i] > envF[q] {
				q = i
			}
		}
		lo := slices.Min(envF[max(prev, q-5*ms) : q+1])
		i90 := q
		for i90 > 0 && envF[i90] >= lo+0.9*(envF[q]-lo) {
			i90--
		}
		i10 := i90
		for i10 > 0 && envF[i10] >= lo+0.1*(envF[q]-lo) {
			i10--
		}
		att = append(att, float64(i90-i10)/rate*1000)
		next := len(env)
		if k+1 < len(peaks) {
			next = peaks[k+1]
		}
		d := p
		for d < next && env[d] >= 0.1*pk {
			d++
		}
		if d >= next {
			cut++
		} else {
			dec = append(dec, float64(d-p)/rate*1000)
		}
		d10 := p
		for d10 < next && env[d10] >= 0.316*pk {
			d10++
		}
		if d10 >= next {
			cut10++
		} else {
			dec10 = append(dec10, float64(d10-p)/rate*1000)
		}
		var e, m float64
		for i := i10; i < d; i++ {
			e += y[i] * y[i]
			m = max(m, math.Abs(y[i]))
		}
		if d > i10 {
			crest = append(crest, 20*math.Log10(m/math.Sqrt(e/float64(d-i10))))
		}
		n := 256
		for float64(n) < 0.006*rate {
			n <<= 1
		}
		a := max(0, i10-ms/2)
		if a+n <= len(y) {
			z := make([]complex128, n)
			for i := range n {
				z[i] = complex(y[a+i]*(0.5-0.5*math.Cos(2*math.Pi*float64(i)/float64(n))), 0)
			}
			analysis.FFT(z)
			var tot, c float64
			for b := 1; b < n/2; b++ {
				f := float64(b) * rate / float64(n)
				if f < 1000 || f > 16000 {
					continue
				}
				q := real(z[b])*real(z[b]) + imag(z[b])*imag(z[b])
				tot += q
				c += f * q
			}
			if tot > 0 {
				cen = append(cen, c/tot)
			}
		}
		if k > 0 {
			if g := float64(p-peaks[k-1]) / rate * 1000; g < 50 {
				st.gaps = append(st.gaps, g)
				if last > 0 {
					r := (last - g) / (last + g)
					lvSum += r * r
					lvN++
				}
				last = g
			} else {
				last = 0
			}
		}
	}
	st.n = len(peaks)
	st.decay10MS = pct(dec10, 0.5)
	st.attackMS, st.decayMS, st.crestDB, st.centroid = pct(att, 0.5), pct(dec, 0.5), pct(crest, 0.5), pct(cen, 0.5)
	if st.n > 0 {
		st.censored = float64(cut) / float64(st.n)
		st.censored10 = float64(cut10) / float64(st.n)
	}
	st.gapP10, st.gapP50, st.gapP90 = pct(st.gaps, 0.1), pct(st.gaps, 0.5), pct(st.gaps, 0.9)
	var mu, sd float64
	for _, g := range st.gaps {
		mu += g
	}
	mu /= float64(len(st.gaps))
	for _, g := range st.gaps {
		sd += (g - mu) * (g - mu)
	}
	sd = math.Sqrt(sd / float64(len(st.gaps)))
	st.gapCV = sd / mu
	st.lv = 3 * lvSum / float64(max(1, lvN))
	st.burst = (sd - mu) / (sd + mu)
	return st
}
