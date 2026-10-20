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

// legacyAM are the overrides that give the noise AM of HEAD a553aef: one
// pole at 2 kHz and the full depth at every RPM.
var legacyAM = []string{"noise.am_cutoff_hz=0", "noise.am_redline=1", "noise.am_idle=1"}

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

// hissHolds renders the holds scenario name of p from dir and measures
// each hold of at least 2.5 s.
func hissHolds(t *testing.T, p Profile, dir, name string) []hissHold {
	t.Helper()
	scn, err := scenario.ParseFile(filepath.Join(dir, name+".scn"))
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
// to 12 kHz level and the least HNR from 20 to 500 Hz, in decibels. With
// the noise AM of HEAD a553aef, the worst holds were at -12.2 and 19.4 dB
// (flat6), -14.6 and 16.1 dB (inline4), -15.2 and 19.8 dB (v8), -14.4
// and 17.1 dB (flat12), -29.4 and 23.4 dB (es-v12-60-aero) and -39.3 and
// 24.1 dB (el-v12-60-aero). The valve tick low pass (tick_lp_hz) raises
// the 4 to 12 kHz level of the top hold of es-v12-60-aero and
// el-v12-60-aero by 1.3 and 1.4 dB, to -37.4 and -48.2 dB. The tick, the
// jet and the intake noise share one noise source, and the low pass
// changes the phase of the tick against the other two.
var hissBounds = map[string]struct{ hf, hnr float64 }{
	"flat12":         {-17.5, 19},
	"flat6":          {-14, 23.5},
	"inline4":        {-15.3, 18.5},
	"v8":             {-24, 22},
	"es-v12-60-aero": {-36.5, 26},
	"el-v12-60-aero": {-47.5, 28},
}

// TestHissHighRPM checks that the noise AM leaves the harmonics clear at
// high RPM, on the holds-t100 scenario. The high holds are the holds from
// 4000 RPM up, or the top hold of an engine with a lower redline. At each
// high hold, the 4 to 12 kHz level and the HNR from 20 to 500 Hz stay
// within hissBounds. Against the noise AM of HEAD a553aef, the 4 to 12
// kHz level is at least 1 dB lower and the HNR at least 1.5 dB higher.
// The log gives the measures of every hold with the HEAD measures beside
// them.
func TestHissHighRPM(t *testing.T) {
	t.Parallel()
	for name, b := range hissBounds {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, dir := hissProfile(t, name)
			q, _ := hissProfile(t, name, legacyAM...)
			now, was := hissHolds(t, p, dir, "holds-t100"), hissHolds(t, q, dir, "holds-t100")
			for i, h := range now {
				w := was[i]
				t.Logf("%5.0f RPM: 4-12 kHz %6.1f dB (was %6.1f), HNR 20-500 Hz %5.1f dB (was %5.1f), floor %6.1f dB (was %6.1f), RMS %+.2f dB",
					h.rpm, h.hf, w.hf, h.hnr, w.hnr, h.floor, w.floor, h.rmsDB-w.rmsDB)
				if h.rpm < 4000 && i < len(now)-1 {
					continue
				}
				if h.hf > b.hf || h.hnr < b.hnr {
					t.Errorf("%.0f RPM: 4-12 kHz %.1f dB, want at most %.1f; HNR 20-500 Hz %.1f dB, want at least %.1f",
						h.rpm, h.hf, b.hf, h.hnr, b.hnr)
				}
				if h.hf > w.hf-1 || h.hnr < w.hnr+1.5 {
					t.Errorf("%.0f RPM: 4-12 kHz %.1f dB and HNR %.1f dB, against %.1f and %.1f dB at HEAD", h.rpm, h.hf, h.hnr, w.hf, w.hnr)
				}
			}
		})
	}
}

// hissPops renders the pop schedule of newPopSchedule on p for seconds,
// with pops and with no pops, and returns the difference: the pops alone.
func hissPops(t *testing.T, p Profile, request bool, seconds float64) []float64 {
	t.Helper()
	a, b := newEngine(t, p), newEngine(t, p)
	sa, sb := newPopSchedule(p, request, 0.5), newPopSchedule(p, request, 0.5)
	n := int(seconds * testRate / 256)
	buf := make([]float32, 256)
	d := make([]float64, 0, n*256)
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
			d[i*256+j] -= float64(v)
		}
	}
	return d
}

// TestHissPops checks that the noise AM change leaves the pops alone. On
// each profile, the requested pops at full throttle and the own pops of a
// lift-off keep their level within 1 dB and their wave, with a
// correlation of at least 0.999, against the noise AM of HEAD a553aef.
// The level changes only with the engine level that the pops follow. The
// output gain is 20 dB lower, so that the output limiter does not act.
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
				x := hissPops(t, load(), request, 10)
				y := hissPops(t, load(legacyAM...), request, 10)
				var xy, xx, yy float64
				for i := range x {
					xy += x[i] * y[i]
					xx += x[i] * x[i]
					yy += y[i] * y[i]
				}
				db := 10 * math.Log10(xx/yy)
				r := xy / math.Sqrt(xx*yy)
				t.Logf("pops %+.2f dB against HEAD, correlation %.4f", db, r)
				if math.Abs(db) > 1 || r < 0.999 {
					t.Errorf("pops %+.2f dB against HEAD with a correlation of %.4f, want within 1 dB and at least 0.999", db, r)
				}
			})
		}
	}
}

// TestHissIdle checks the noise AM at idle and at 2000 RPM against the
// noise AM of HEAD a553aef, where am_idle 0.5 halves the AM depth at idle.
// At the idle and the 2000 RPM holds of the holds-t000, holds-t050 and
// holds-t100 scenarios, the 4 to 12 kHz level and the floor are not higher
// by more than 0.2 dB, the HNR from 20 to 500 Hz is not lower by more
// than 0.2 dB, and the RMS level is within 0.3 dB. On holds-t100, the
// floor is at least 3.5 dB lower at both holds. With am_idle 0.5, it was
// 4.2 to 5.3 dB lower at idle and 4.8 to 6.2 dB lower at 2000 RPM, and
// the RMS level changed by -0.04 to -0.26 dB.
func TestHissIdle(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		for _, scn := range []string{"holds-t000", "holds-t050", "holds-t100"} {
			t.Run(name+"/"+scn, func(t *testing.T) {
				t.Parallel()
				p, dir := hissProfile(t, name)
				q, _ := hissProfile(t, name, legacyAM...)
				now, was := hissHolds(t, p, dir, scn), hissHolds(t, q, dir, scn)
				for i, h := range now[:2] {
					w := was[i]
					d := hissHold{rpm: h.rpm, hnr: h.hnr - w.hnr, floor: h.floor - w.floor, hf: h.hf - w.hf, rmsDB: h.rmsDB - w.rmsDB}
					t.Logf("%5.0f RPM: change of 4-12 kHz %+.2f dB, HNR 20-500 Hz %+.2f dB, floor %+.2f dB, RMS %+.3f dB", d.rpm, d.hf, d.hnr, d.floor, d.rmsDB)
					worse := d.hf > 0.2 || d.floor > 0.2 || d.hnr < -0.2 || math.Abs(d.rmsDB) > 0.3
					if worse || scn == "holds-t100" && d.floor > -3.5 {
						t.Errorf("%.0f RPM: 4-12 kHz %+.2f dB, HNR %+.2f dB, floor %+.2f dB, RMS %+.3f dB against HEAD", d.rpm, d.hf, d.hnr, d.floor, d.rmsDB)
					}
				}
			})
		}
	}
}
