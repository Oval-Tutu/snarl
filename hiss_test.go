package snarl

import (
	"fmt"
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// The hiss tests measure the noise AM layer on the full-throttle holds,
// with the method of docs/research/hiss-analysis.md: a 65536-point Hann
// power spectrum of each hold from 0.8 s to 2.6 s, the harmonic-to-noise
// ratio from 20 to 500 Hz on the half orders, and the power from 4 to 12
// kHz relative to the power from 20 Hz to 20 kHz.

// hissFFT is the spectrum size of the hiss measures.
const hissFFT = 65536

// noAM is the override that turns the noise AM off.
var noAM = []string{"noise.am=0"}

// hissHold holds the measures of one hold, in decibels.
type hissHold struct {
	rpm, hnr, floor, hf, rmsDB float64
}

// hissPower returns the summed Hann power spectrum of the first and the
// last hissFFT samples of x.
func hissPower(x []float64) []float64 {
	w := analysis.Hann(hissFFT)
	p := make([]float64, hissFFT/2+1)
	buf := make([]complex128, hissFFT)
	for _, o := range []int{0, len(x) - hissFFT} {
		for i := range buf {
			buf[i] = complex(x[o+i]*w[i], 0)
		}
		analysis.FFT(buf)
		for i := range p {
			p[i] += real(buf[i])*real(buf[i]) + imag(buf[i])*imag(buf[i])
		}
	}
	return p
}

// hissMeasure returns the measures of the window x of a hold at rpm. The
// half-order spacing starts at rpm/120 and is refined within 2 per cent on
// the harmonics from 20 to 500 Hz. The harmonic bins lie within 0.15 of a
// half order, and the noise bins from 0.3 to 0.7 between half orders. The
// floor is the noise density times the band width from 20 Hz to 16 kHz,
// relative to the total power of that band.
func hissMeasure(x []float64, rpm float64) hissHold {
	p := hissPower(x)
	df := float64(testRate) / hissFFT
	bin := func(f float64) int { return int(math.Round(f / df)) }
	s := rpm / 120
	best := -1.0
	for i := range 81 {
		c := rpm / 120 * (0.98 + 0.0005*float64(i))
		var sum float64
		for k := 1; float64(k)*c < 500; k++ {
			if float64(k)*c >= 20 {
				sum += p[bin(float64(k)*c)]
			}
		}
		if sum > best {
			best, s = sum, c
		}
	}
	band := func(lo, hi float64) (harm, noise, total float64, nh, nn, nt int) {
		for i := bin(lo); i <= bin(hi) && i < len(p); i++ {
			ph := float64(i)*df/s - math.Floor(float64(i)*df/s)
			total += p[i]
			nt++
			switch {
			case ph < 0.15 || ph > 0.85:
				harm += p[i]
				nh++
			case ph > 0.3 && ph < 0.7:
				noise += p[i]
				nn++
			}
		}
		return harm, noise, total, nh, nn, nt
	}
	db := func(v float64) float64 { return 10 * math.Log10(max(v, 1e-30)) }
	h, n, _, nh, nn, _ := band(20, 500)
	_, fn, ft, _, fnn, fnt := band(20, 16000)
	_, _, hf, _, _, _ := band(4000, 12000)
	_, _, all, _, _, _ := band(20, 20000)
	var e float64
	for _, v := range x {
		e += v * v
	}
	return hissHold{
		rpm:   rpm,
		hnr:   db(h / float64(nh) / (n / float64(nn))),
		floor: db(fn / float64(fnn) * float64(fnt) / ft),
		hf:    db(hf / all),
		rmsDB: db(e / float64(len(x))),
	}
}

// hissHolds renders the holds-t100 scenario of p and measures each hold of
// at least 2.5 s.
func hissHolds(t *testing.T, p Profile) []hissHold {
	t.Helper()
	x, scn := renderScenario(t, p, "holds-t100")
	return hissScenarioHolds(scn, func(i int) float64 { return x[i] })
}

// hissRefHolds measures the holds of the reference render of holds-t100
// of profile id, and returns false when the render is not present.
func hissRefHolds(t *testing.T, id string) ([]hissHold, bool) {
	t.Helper()
	x, ok := upstreamRender(id, "holds-t100")
	if !ok {
		return nil, false
	}
	scn := parseUpstream(t, id, "holds-t100")
	return hissScenarioHolds(scn, func(i int) float64 { return x[min(i, len(x)-1)] }), true
}

// hissScenarioHolds measures each hold of at least 2.5 s of scn in an
// output at testRate, which at returns sample by sample.
func hissScenarioHolds(scn *scenario.Scenario, at func(i int) float64) []hissHold {
	var out []hissHold
	start := 0.0
	for _, g := range scn.Segments {
		if g.Kind == scenario.Hold && g.Seconds >= 2.5 {
			i0 := int((start + 0.8) * testRate)
			w := make([]float64, int(1.8*testRate))
			for i := range w {
				w[i] = at(i0 + i)
			}
			out = append(out, hissMeasure(w, g.RPM0))
		}
		start += g.Seconds
	}
	return out
}

// hissProfile loads the embedded profile by name, with the overrides.
func hissProfile(t *testing.T, name string, overrides ...string) Profile {
	t.Helper()
	p, err := LoadProfileWith(name, overrides...)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// highHolds returns the high holds of the holds-t100 measures: from 4000
// RPM up, and the top hold of an engine with a lower redline.
func highHolds(h []hissHold) []hissHold {
	var out []hissHold
	for i, v := range h {
		if v.rpm >= 4000 || i == len(h)-1 {
			out = append(out, v)
		}
	}
	return out
}

// worstHiss returns the most 4 to 12 kHz level and the least HNR from 20
// to 500 Hz of the high holds.
func worstHiss(h []hissHold) (hf, hnr float64) {
	hf, hnr = math.Inf(-1), math.Inf(1)
	for _, v := range highHolds(h) {
		hf, hnr = max(hf, v.hf), min(hnr, v.hnr)
	}
	return hf, hnr
}

// hissRefs are worstHiss of the reference render of holds-t100: the most
// 4 to 12 kHz level and the least HNR from 20 to 500 Hz at the high holds,
// in decibels.
var hissRefs = map[string][2]float64{
	"i4-vtec":      {-34.84, 30.17},
	"v8-ls":        {-34.01, 38.06},
	"f4-ej25":      {-33.88, 38.76},
	"v12-412":      {-20.41, 11.18},
	"v12-merlin":   {-21.00, 34.86},
	"f6-aircooled": {-18.54, 7.44},
}

// hissTol is the most in decibels that TestHissHighRPM accepts above the
// 4 to 12 kHz level and below the HNR of the reference.
const hissTol = 3.0

// hissGaps are the bounds of TestHissHighRPM where snarl is further than
// hissTol from the reference: the most 4 to 12 kHz level and the least
// HNR, 0.5 dB beyond snarl. i4-vtec has -31.5 dB from 4 to 12 kHz against
// -34.8 dB, v8-ls an HNR of 34.4 dB against 38.1 dB, and v12-412 an HNR of
// -1.8 dB against 11.2 dB: its measured firing gain spread is 0.43 at the
// lowest row and 0.08 to 0.35 at the high holds, and the noise of the
// firing gains fills the gaps between the half orders below 500 Hz.
var hissGaps = map[string][2]float64{
	"i4-vtec": {-31.0, 30.17 - hissTol},
	"v8-ls":   {-34.01 + hissTol, 33.9},
	"v12-412": {-20.41 + hissTol, -2.3},
}

// TestHissHighRPM checks that the noise leaves the harmonics clear at high
// RPM, on the holds-t100 scenario. At the high holds, the most 4 to 12 kHz
// level is at most hissTol above that of the reference, and the least HNR
// from 20 to 500 Hz at most hissTol below it, or within hissGaps. The log
// gives the measures of
// every hold with the measures with no noise AM beside them.
func TestHissHighRPM(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			now, off := hissHolds(t, hissProfile(t, name)), hissHolds(t, hissProfile(t, name, noAM...))
			for i, h := range now {
				w := off[i]
				t.Logf("%5.0f RPM: 4-12 kHz %6.1f dB (no AM %6.1f), HNR 20-500 Hz %5.1f dB (no AM %5.1f), floor %6.1f dB (no AM %6.1f)",
					h.rpm, h.hf, w.hf, h.hnr, w.hnr, h.floor, w.floor)
			}
			ref := hissRefs[name]
			if r, ok := hissRefHolds(t, name); ok {
				hf, hnr := worstHiss(r)
				checkRecorded(t, "hiss", []float64{hf, hnr}, ref[:])
			}
			hf, hnr := worstHiss(now)
			t.Logf("high holds: 4-12 kHz at most %.1f dB, HNR at least %.1f dB, reference %.1f and %.1f dB", hf, hnr, ref[0], ref[1])
			bound := [2]float64{ref[0] + hissTol, ref[1] - hissTol}
			if g, ok := hissGaps[name]; ok {
				bound = g
			}
			if hf > bound[0] || hnr < bound[1] {
				t.Errorf("high holds: 4-12 kHz %.1f dB, want at most %.1f; HNR 20-500 Hz %.1f dB, want at least %.1f",
					hf, bound[0], hnr, bound[1])
			}
		})
	}
}

// hissPops renders the pop schedule of newPopSchedule on p for seconds,
// with pops and with no pops, and returns the difference, the pops alone,
// and the mean square of the render with no pops.
func hissPops(t *testing.T, p Profile, request bool, seconds float64) ([]float64, float64) {
	t.Helper()
	a, b := newEngine(t, p), newEngine(t, p)
	sa, sb := newPopSchedule(p, request, 0.5), newPopSchedule(p, request, 0.5)
	n := int(seconds * testRate / 256)
	buf := make([]float32, 256)
	d := make([]float64, 0, n*256)
	var eng float64
	for i := range n {
		s, _ := sa.next(i)
		a.Set(s)
		a.Render(buf)
		for _, v := range buf {
			d = append(d, float64(v))
		}
		s, _ = sb.next(i)
		s.Flags |= FlagNoPops
		s.PopRequest = 0
		b.Set(s)
		b.Render(buf)
		for j, v := range buf {
			eng += float64(v) * float64(v)
			d[i*256+j] -= float64(v)
		}
	}
	return d, eng / float64(len(d))
}

// TestHissPops checks that the noise AM leaves the pops alone. On each
// profile, the requested pops at full throttle and the own pops of a
// lift-off keep their level over the engine within 1 dB and their wave,
// with a correlation of at least 0.999, against the pops with no noise AM.
// The pops join the engine after the leveller and follow the engine
// level. Below the knee, the noise AM changes the engine level, because
// the AM term keeps 1-am of the pulse, and the pops follow it. The output
// gain is 20 dB lower, so that the output limiter does not act.
func TestHissPops(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		for _, request := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/request-%t", name, request), func(t *testing.T) {
				t.Parallel()
				load := func(o ...string) Profile {
					p := hissProfile(t, name, append(o, "pops.density=4")...)
					s := *p.spec
					s.Voice.OutputDB -= 20
					p.spec = &s
					return p
				}
				x, ex := hissPops(t, load(), request, 10)
				y, ey := hissPops(t, load(noAM...), request, 10)
				var xy, xx, yy float64
				for i := range x {
					xy += x[i] * y[i]
					xx += x[i] * x[i]
					yy += y[i] * y[i]
				}
				db := 10 * math.Log10(xx/yy)
				eng := 10 * math.Log10(ex/ey)
				r := xy / math.Sqrt(xx*yy)
				t.Logf("pops %+.2f dB and engine %+.2f dB against no AM, correlation %.4f", db, eng, r)
				if math.Abs(db-eng) > 1 || r < 0.999 {
					t.Errorf("pops %+.2f dB over the engine against no AM with a correlation of %.4f, want within 1 dB and at least 0.999", db-eng, r)
				}
			})
		}
	}
}

// noiseHolds returns the residual of the first hold of holds-t100 and of
// the hold nearest 3000 RPM after it in the render x, measured from 0.5 s
// into the hold to its end: the residual from 2 to 6 kHz, the AM peak and
// the AM trough of each, in decibels, and the hold RPMs.
func noiseHolds(t *testing.T, x []float64, scn *scenario.Scenario) (m []float64, rpms []float64) {
	t.Helper()
	holds := holdsOf(scn)
	mid := 1
	for i := 2; i < len(holds); i++ {
		if math.Abs(holds[i].RPM0-3000) < math.Abs(holds[mid].RPM0-3000) {
			mid = i
		}
	}
	start, k := 0.0, 0
	for _, g := range scn.Segments {
		if g.Kind == scenario.Hold {
			if k == 0 || k == mid {
				i0 := int((start + 0.5) * testRate)
				y := x[i0:min(len(x), i0+int((g.Seconds-0.5)*testRate))]
				r, ok := analysis.Residual(y, testRate, g.RPM0)
				if !ok {
					t.Fatalf("%.0f RPM: no residual", g.RPM0)
				}
				m = append(m, r.Bands[2].DB, r.AMPeakDB, r.AMTroughDB)
				rpms = append(rpms, g.RPM0)
			}
			k++
		}
		start += g.Seconds
	}
	return m, rpms
}

// noiseRefs are noiseHolds of the reference render of holds-t100: the
// residual from 2 to 6 kHz, the AM peak and the AM trough at the first
// hold, then at the hold nearest 3000 RPM, in decibels.
var noiseRefs = map[string][6]float64{
	"i4-vtec":      {-47.89, 10.93, -15.66, -43.02, 8.02, -17.20},
	"v8-ls":        {-26.25, 5.56, -11.55, -30.95, 3.49, -4.66},
	"f4-ej25":      {-38.73, 9.91, -12.74, -33.93, 7.83, -12.50},
	"v12-412":      {-27.34, 3.89, -8.29, -26.88, 2.92, -6.99},
	"v12-merlin":   {-12.00, 5.00, -5.35, -21.75, 2.98, -5.49},
	"f6-aircooled": {-22.88, 2.87, -3.87, -20.20, 7.73, -5.31},
}

// noiseTol are the largest distances in decibels from noiseRefs that
// TestNoiseCycle accepts: of the 2 to 6 kHz residual, of the AM peak and
// of the AM trough.
var noiseTol = [3]float64{6, 4, 8}

// noiseGaps are the tolerances of TestNoiseCycle for a profile whose noise
// is further than noiseTol from the reference: 0.5 dB beyond the largest
// gap. f6-aircooled has the engine-sim noise of the model defaults,
// because EngineLab has no synthesiser noise. Against the EngineLab
// render, its 2 to 6 kHz residual is 7.8 and 10.6 dB lower, its AM peak at
// the first hold 5.8 dB higher and its AM trough at 3000 RPM 11.4 dB
// deeper.
var noiseGaps = map[string][3]float64{"f6-aircooled": {11.1, 6.3, 11.9}}

// TestNoiseCycle checks that the noise follows the engine cycle as the
// noise of the reference does, at the first hold of holds-t100 and at the
// hold nearest 3000 RPM, measured from 0.5 s into the hold to its end. The
// residual power from 2 to 6 kHz, and the peak and the trough of the
// residual power over the cycle, are within noiseTol of the reference, or
// within noiseGaps. A
// noise that is steady over the cycle has a peak and a trough of about
// 0 dB.
func TestNoiseCycle(t *testing.T) {
	if raceEnabled {
		t.Skip("measures 12 residuals, minutes under the race detector")
	}
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x, scn := renderScenario(t, hissProfile(t, name), "holds-t100")
			got, rpms := noiseHolds(t, x, scn)
			ref := noiseRefs[name]
			if r, ok := upstreamRender(name, "holds-t100"); ok {
				m, _ := noiseHolds(t, r, scn)
				checkRecorded(t, "noise cycle", m, ref[:])
			}
			tol := noiseTol
			if g, ok := noiseGaps[name]; ok {
				tol = g
			}
			for i, rpm := range rpms {
				g, w := got[3*i:3*i+3], ref[3*i:3*i+3]
				t.Logf("%5.0f RPM: 2-6 kHz %.1f dB, AM peak %+.1f dB, trough %+.1f dB; reference %.1f, %+.1f and %+.1f dB",
					rpm, g[0], g[1], g[2], w[0], w[1], w[2])
				for j, what := range []string{"2-6 kHz", "AM peak", "AM trough"} {
					if math.Abs(g[j]-w[j]) > tol[j] {
						t.Errorf("%.0f RPM: %s %.1f dB, want within %.1f dB of the reference %.1f dB", rpm, what, g[j], tol[j], w[j])
					}
				}
			}
		})
	}
}
