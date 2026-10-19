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
	s := DefaultSpec()
	s.Name = "p"
	s.IdleRPM, s.RedlineRPM = 800, 6800
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, Bank: 0, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, PipeRadiusM: 0.025, OutletM: 0.025, WallLossDB: 2, WallLossHz: 2500, ChamberM: 0.4, ChamberArea: 8, Gain: 1}}
	s.Exhaust.OutletDirect = 0.3
	s.Pops.Density = 0.4
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
	y.pops.fire(1, 1, kindPop)
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
// pops and in a dense pop state, where the heat and the run probability
// stay at 1 on overrun at full pop density.
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
				c = Control{RPM: 6000, Throttle: 1, Load: 1, Running: true, Gear: 3}
				y.Render(buf, c)
				c = Control{RPM: 6000, Running: true, Overrun: true, Gear: 3}
			}
			b.ReportAllocs()
			for b.Loop() {
				if dense {
					y.pops.heat, y.pops.prob = 1, 1
				}
				y.Render(buf, c)
			}
		})
	}
}

// fired returns the number of pops started.
func (p *pops) fired() int {
	var n int
	for _, c := range p.count {
		n += c
	}
	return n
}

// popSpec is a one-cylinder engine with one path for isolated pops.
func popSpec() Spec {
	s := DefaultSpec()
	s.Name = "p"
	s.IdleRPM, s.RedlineRPM = 800, 6800
	s.Cylinders = []CylinderSpec{{CrankDeg: 0, Bank: 0, PrimaryM: 0.4}}
	s.Paths = []PathSpec{{CollectorM: 1.3, Reflection: -0.4, DampingHz: 3000, PipeRadiusM: 0.025, OutletM: 0.025, WallLossDB: 2, WallLossHz: 2500, ChamberM: 0.4, ChamberArea: 8, Gain: 1}}
	s.Exhaust.OutletDirect = 0.3
	s.Pops.Density = 0.4
	return s
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
	y.pops.fire(1, 1, kind)
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
// limits are those of TestPopShape. The burble is 14 to 30 dB below the
// pop, as in the pack.
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
		if k == kindBurble && (pop-mp < 14 || pop-mp > 30) {
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

// TestPopMix checks that the kind picks follow the profile weights within
// 1.5 percentage points over 20000 picks, and that the pops of 60 s of
// overrun runs never play a kind with weight 0.
func TestPopMix(t *testing.T) {
	for _, w := range [][numKinds]float64{{0.1, 0.2, 0.3, 0.4}, {0.5, 0, 0.5, 0}} {
		s := DefaultSpec()
		s.IdleRPM, s.RedlineRPM = 800, 6800
		s.Pops.Density = 1
		s.Pops.WeightPop, s.Pops.WeightBoom, s.Pops.WeightCrackle, s.Pops.WeightBurble = w[0], w[1], w[2], w[3]
		p := newPops(&s, rate, chunk/float64(rate))
		var picks [numKinds]int
		for range 20000 {
			picks[p.pick()]++
		}
		for k := range numKinds {
			if got := float64(picks[k]) / 20000; math.Abs(got-w[k]) > 0.015 || (w[k] == 0 && picks[k] > 0) {
				t.Errorf("weights %v: kind %d share %.3f", w, k, got)
			}
		}
		for range 60 {
			p.heat = 1
			p.control(4000, 1, false, false, 1)
			for i := range rate {
				if i%chunk == 0 {
					p.control(4000, 0, true, false, 1)
				}
				p.tick(2)
			}
			p.control(4000, 0, false, false, 1)
		}
		if p.fired() < 100 {
			t.Errorf("weights %v: %d pops in 60 s", w, p.fired())
		}
		for k := range numKinds {
			if w[k] == 0 && p.count[k] > 0 {
				t.Errorf("weights %v: %d pops of kind %d", w, p.count[k], k)
			}
		}
	}
}

// TestPopCap checks that at most maxPopEvents pops sound at once, that a
// pop with every slot busy does not start, and that the path jumps return
// to zero when the last pop ends.
func TestPopCap(t *testing.T) {
	s := DefaultSpec()
	p := newPops(&s, rate, chunk/float64(rate))
	p.heat = 1
	for i := range maxPopEvents + 3 {
		e := p.fire(2, 1, kindCrackle)
		if (e == nil) != (i >= maxPopEvents) {
			t.Fatalf("pop %d: started %v with %d active", i, e != nil, p.active)
		}
	}
	if p.active != maxPopEvents || p.fired() != maxPopEvents {
		t.Fatalf("%d active and %d started, want %d", p.active, p.fired(), maxPopEvents)
	}
	p.bang(2)
	if p.fired() != maxPopEvents {
		t.Error("a bang started with every slot busy")
	}
	for range 2 * rate {
		p.tick(2)
		if p.active > maxPopEvents {
			t.Fatalf("%d active", p.active)
		}
	}
	if p.active != 0 || p.jump != [MaxBanks]float32{} {
		t.Errorf("%d active after 2 s, jumps %v", p.active, p.jump)
	}
}
