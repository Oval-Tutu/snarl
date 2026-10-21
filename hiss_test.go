package snarl

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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

// hissHolds renders the holds-t100 scenario of p from dir and measures
// each hold of at least 2.5 s.
func hissHolds(t *testing.T, p Profile, dir string) []hissHold {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join(dir, "holds-t100.scn"))
	if err != nil {
		t.Fatal(err)
	}
	x := render(newEngine(t, p), scn.Duration(), func(t float64) Snapshot {
		rpm, thr := scn.At(t)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		return s
	})
	return hissScenarioHolds(scn, func(i int) float64 { return float64(x[i]) })
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

// hissProfile loads a tuned profile or a catalogue profile by name, with
// the overrides, and returns it with its scenario directory.
func hissProfile(t *testing.T, name string, overrides ...string) (Profile, string) {
	t.Helper()
	if _, err := LoadProfile(name); err == nil {
		p, err := LoadProfileWith(name, overrides...)
		if err != nil {
			t.Fatal(err)
		}
		return p, filepath.Join("scenarios", name)
	}
	data, err := os.ReadFile(filepath.Join("profiles", "catalogue", name+".toml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseProfileWith(data, overrides...)
	if err != nil {
		t.Fatal(err)
	}
	return p, filepath.Join("scenarios", "catalogue", name)
}

// hissBounds are the bounds at the high full-throttle holds: the most 4
// to 12 kHz level and the least HNR from 20 to 500 Hz, in decibels. The
// engine-sim input jitter puts noise from 4 to 12 kHz at the level of the
// engine-sim references: at the high holds, the engine-sim 4 to 12 kHz
// level is at most -26.6 dB (flat12), -39.2 dB (flat6), -35.0 dB (inline4),
// -34.3 dB (v8) and -21.0 dB (es-v12-60-aero, at its top hold). The worst
// snarl holds are -25.0 and 26.2 dB (flat12), -36.8 and 36.6 dB (flat6),
// -31.5 and 19.0 dB (inline4), -28.8 and 24.8 dB (v8), -21.0 and 31.7 dB
// (es-v12-60-aero) and -33.5 and 30.3 dB (el-v12-60-aero). The bounds are
// about 1 dB from them. The v8 hold at 5000 RPM is 5.5 dB above engine-sim,
// because order 12 meets the narrow formant at 1 kHz. The inline4 HNR
// falls at 8000 RPM and up, where the jitter noise of its single path
// meets a pulse train with a high crest factor.
var hissBounds = map[string]struct{ hf, hnr float64 }{
	"flat12":         {-24, 25},
	"flat6":          {-35.5, 35.5},
	"inline4":        {-30.5, 18},
	"v8":             {-28, 24},
	"es-v12-60-aero": {-20, 30.5},
	"el-v12-60-aero": {-32.5, 29},
}

// TestHissHighRPM checks that the noise leaves the harmonics clear at high
// RPM, on the holds-t100 scenario. The high holds are the holds from 4000
// RPM up, or the top hold of an engine with a lower redline. At each high
// hold, the 4 to 12 kHz level and the HNR from 20 to 500 Hz stay within
// hissBounds. The log gives the measures of every hold with the measures
// with no noise AM beside them.
func TestHissHighRPM(t *testing.T) {
	t.Parallel()
	for name, b := range hissBounds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, dir := hissProfile(t, name)
			q, _ := hissProfile(t, name, noAM...)
			now, off := hissHolds(t, p, dir), hissHolds(t, q, dir)
			for i, h := range now {
				w := off[i]
				t.Logf("%5.0f RPM: 4-12 kHz %6.1f dB (no AM %6.1f), HNR 20-500 Hz %5.1f dB (no AM %5.1f), floor %6.1f dB (no AM %6.1f)",
					h.rpm, h.hf, w.hf, h.hnr, w.hnr, h.floor, w.floor)
				if h.rpm < 4000 && i < len(now)-1 {
					continue
				}
				if h.hf > b.hf || h.hnr < b.hnr {
					t.Errorf("%.0f RPM: 4-12 kHz %.1f dB, want at most %.1f; HNR 20-500 Hz %.1f dB, want at least %.1f",
						h.rpm, h.hf, b.hf, h.hnr, b.hnr)
				}
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
					p, _ := hissProfile(t, name, append(o, "pops.density=4")...)
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

// noiseTargets are the engine-sim residual from 2 to 6 kHz relative to the
// whole signal, in decibels, at the idle hold and the 3000 RPM hold of
// holds-t100 (docs/research/engine-sim-target.md section 5). In that band,
// the output is the noise AM alone.
var noiseTargets = map[string][2]float64{
	"flat12":  {-28.4, -29.8},
	"flat6":   {-25.2, -33.9},
	"inline4": {-48.2, -43.0},
	"v8":      {-26.4, -31.2},
}

// TestNoiseCycle checks that the noise follows the engine cycle as the
// engine-sim noise does, at the idle hold and the 3000 RPM hold of
// holds-t100, measured from 0.5 s into the hold to its end. The residual
// power of the noise over the cycle has a peak 2 to 12 dB above its mean
// and a trough 3 to 24 dB below it: engine-sim has 2.8 to 11.2 dB and 4.1
// to 18.3 dB. A noise that is steady over the cycle has about 0 dB for
// both. The residual from 2 to 6 kHz is within 3 dB of noiseTargets at idle
// and within 6 dB at 3000 RPM. The AM depth, the derivative mix and the
// input jitter are the engine-sim values, so the noise is up to 2.8 dB
// below engine-sim at idle (flat12), where the AM depth of 1 has no room
// to rise.
func TestNoiseCycle(t *testing.T) {
	t.Parallel()
	for name, want := range noiseTargets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, dir := hissProfile(t, name)
			scn, err := scenario.ParseFile(filepath.Join(dir, "holds-t100.scn"))
			if err != nil {
				t.Fatal(err)
			}
			x := render(newEngine(t, p), scn.Duration(), func(t float64) Snapshot {
				rpm, thr := scn.At(t)
				return Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
			})
			start, i := 0.0, 0
			for _, g := range scn.Segments {
				if g.Kind == scenario.Hold && (i == 0 || g.RPM0 == 3000) {
					y := make([]float64, int((g.Seconds-0.5)*testRate))
					for j := range y {
						y[j] = float64(x[int((start+0.5)*testRate)+j])
					}
					r, ok := analysis.Residual(y, testRate, g.RPM0)
					if !ok {
						t.Fatalf("%.0f RPM: no residual", g.RPM0)
					}
					band := r.Bands[2].DB
					tol := 3.0
					if i > 0 {
						tol = 6
					}
					t.Logf("%5.0f RPM: residual %.1f dB, 2-6 kHz %.1f dB (engine-sim %.1f), AM peak %+.1f dB, trough %+.1f dB",
						g.RPM0, r.ResidualDB, band, want[i], r.AMPeakDB, r.AMTroughDB)
					if r.AMPeakDB < 2 || r.AMPeakDB > 12 || r.AMTroughDB > -3 || r.AMTroughDB < -24 || math.Abs(band-want[i]) > tol {
						t.Errorf("%.0f RPM: AM peak %+.1f dB, trough %+.1f dB and 2-6 kHz %.1f dB, want 2 to 12, -3 to -24 and %.1f within %.0f",
							g.RPM0, r.AMPeakDB, r.AMTroughDB, band, want[i], tol)
					}
					i++
				}
				start += g.Seconds
			}
			if i != 2 {
				t.Fatalf("%d holds measured, want 2", i)
			}
		})
	}
}
