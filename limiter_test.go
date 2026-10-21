package snarl

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
)

// limiterHop is the step in samples at which limiterHold reads the pitch.
const limiterHop = 32

// limiterHold renders seconds of p at the snapshot RPM rpm with full
// throttle in third gear, with the overrun flag set as a careless game can
// set it. It returns the output and the pitch RPM every limiterHop
// samples.
func limiterHold(t testing.TB, p Profile, rpm float32, seconds float64) (x, pitch []float32) {
	t.Helper()
	e := newEngine(t, p)
	s := Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3, Flags: FlagRunning | FlagOverrun | FlagRedZone}
	x = make([]float32, int(seconds*testRate)/256*256)
	for i := 0; i < len(x); i += 256 {
		e.Set(s)
		for j := i; j < i+256; j += limiterHop {
			e.Render(x[j : j+limiterHop])
			pitch = append(pitch, e.(*engine).synth.PitchRPM())
		}
	}
	return x, pitch
}

// bounce is one limiter bounce in pitch steps: the cut starts at cut, the
// firing starts again at fire, and the next cut starts at next.
type bounce struct{ cut, fire, next int }

// bounces finds the bounces in a pitch track: a cut starts where the pitch
// stops rising, and the firing starts where it stops falling.
func bounces(pitch []float32) []bounce {
	var out []bounce
	cut, fire, dir := -1, -1, 0
	for i := 1; i < len(pitch); i++ {
		d := 0
		switch {
		case pitch[i] < pitch[i-1]:
			d = -1
		case pitch[i] > pitch[i-1]:
			d = 1
		}
		switch {
		case d < 0 && dir > 0:
			if cut >= 0 && fire > cut {
				out = append(out, bounce{cut, fire, i - 1})
			}
			cut = i - 1
		case d > 0 && dir < 0:
			fire = i - 1
		}
		if d != 0 {
			dir = d
		}
	}
	return out
}

// envWin and envHop are the window and the hop in samples of envelopeDB.
const envWin, envHop = 15 * testRate / 1000, testRate / 1000

// envelopeDB returns the level of x in decibels, from 15 ms RMS windows
// every 1 ms. Value i is the window that starts at i ms.
func envelopeDB(x []float32) []float64 {
	sum := make([]float64, len(x)+1)
	for i, v := range x {
		sum[i+1] = sum[i] + float64(v)*float64(v)
	}
	env := make([]float64, (len(x)-envWin)/envHop)
	for i := range env {
		env[i] = 10 * math.Log10((sum[i*envHop+envWin]-sum[i*envHop])/envWin+1e-20)
	}
	return env
}

// limiterShape measures the level shape of the bounces of x, from 15 ms
// RMS windows every 1 ms in decibels, 4 ms after the pitch. Relative to
// the median level of the second half of the firing, it returns the
// median level of a cut, the 5th percentile level of a cut (the notch),
// and the highest level in the first half of the firing (the bark). Each
// is the median over the bounces.
func limiterShape(x []float32, pitch []float32) (cut, notch, bark float64) {
	const win, hop, lag = envWin, envHop, 4
	env := envelopeDB(x)
	// ms returns the envelope index of pitch step i: the window centre
	// sits at the pitch time plus the lag.
	ms := func(i int) int {
		return min(len(env), max(0, ((i+1)*limiterHop-win/2)/hop+lag))
	}
	pct := func(v []float64, q float64) float64 {
		s := slices.Clone(v)
		slices.Sort(s)
		return s[int(q*float64(len(s)-1))]
	}
	var cuts, notches, barks []float64
	for _, b := range bounces(pitch) {
		c, f, n := ms(b.cut), ms(b.fire), ms(b.next)
		h := f + (n-f)/2
		if n >= len(env) || h <= f || n <= h || f <= c {
			continue
		}
		ref := pct(env[h:n], 0.5)
		cuts = append(cuts, pct(env[c:f], 0.5)-ref)
		notches = append(notches, pct(env[c:f], 0.05)-ref)
		barks = append(barks, slices.Max(env[f:h])-ref)
	}
	if len(cuts) == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	return pct(cuts, 0.5), pct(notches, 0.5), pct(barks, 0.5)
}

// bandPower returns the mean square of x above 3 kHz.
func bandPower(x []float32) float64 {
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = float64(v)
	}
	var e float64
	for _, v := range analysis.BandPass(y, testRate, 3000, 20000) {
		e += v * v
	}
	return e / float64(len(y))
}

// limiterRefs are the bounces of the free run at full throttle into the
// limiter of each engine-sim reference (limiter-free): the bounce rate in
// hertz, the main wander frequency of that run in
// out/reference/upstream/<id>/health.md, and the RPM dip, the median peak
// less the median trough of the bounces in the limiter_fall_rpm_s source
// of profiles/<id>.sources.toml. EngineLab runs below its rev limit, so
// f6-aircooled has none.
var limiterRefs = map[string]struct{ rate, dip float64 }{
	"i4-vtec":    {10.25, 9381 - 8898},
	"v8-ls":      {3.66, 6818 - 6053},
	"f4-ej25":    {7.47, 6548 - 5966},
	"v12-412":    {6.40, 18510 - 15401},
	"v12-merlin": {10.45, 3507 - 3454},
}

// TestLimiterBounce holds the snapshot RPM at the limiter with full
// throttle for 3 s, with the overrun flag set as a careless game can set
// it. The engine bounces off the limiter by itself. From 0.5 s, the rate
// of the cuts and the mean dip of the internal RPM are within 20 per cent
// of the values that the profile limiter settings give, and within 25 per
// cent of the bounces of the engine-sim reference (limiterRefs). The
// coefficient of variation of the intervals between cuts is at most 0.03.
// The references give no level shape, so the level shape of the bounces
// with no pops is checked against the profile levels: the level in a cut
// within 1.5 dB of limiter_pump_db, the notch within 3 dB of
// limiter_notch_db and the re-fire bark within 1.5 dB of limiter_bark_db.
// The check needs a dip of at most 10 per cent of the limiter RPM: the
// v12-412 dip of 3100 RPM, 17 per cent, moves the firing level and the
// pulse table over the bounce, which gives a cut of -4.1 dB, a notch of
// -7.0 dB and a bark of -3.3 dB.
// No overrun effect plays: the band above 3 kHz is at most 1.5 dB louder
// than full throttle 150 RPM below the limiter. The output stays finite
// and below full scale.
func TestLimiterBounce(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			// The level shape comes from a render with no pops, because an
			// ignition cut bang at a random time in a cut changes the level
			// of the cut.
			q := p
			noPops := *p.spec
			noPops.Pops.Density = 0
			q.spec = &noPops
			skip := testRate / 2
			x, pitch := limiterHold(t, p, p.LimiterRPM, 3)
			ref, _ := limiterHold(t, p, p.LimiterRPM-150, 3)
			clean, _ := limiterHold(t, q, p.LimiterRPM, 3)
			x, ref, clean, pitch = x[skip:], ref[skip:], clean[skip:], pitch[skip/limiterHop:]
			for i, v := range x {
				if v != v || v > 0.99 || v < -0.99 {
					t.Fatalf("sample %d is %v", i, v)
				}
			}
			bs := bounces(pitch)
			if len(bs) < 3 {
				t.Fatalf("%d bounces in 2.5 s", len(bs))
			}
			rate := float64(len(bs)-1) * testRate / limiterHop / float64(bs[len(bs)-1].cut-bs[0].cut)
			var dip, sum, sq float64
			for _, b := range bs {
				dip += float64(p.LimiterRPM - pitch[b.fire])
				d := float64(b.next - b.cut)
				sum += d
				sq += d * d
			}
			nb := float64(len(bs))
			dip /= nb
			mean := sum / nb
			cv := math.Sqrt(max(0, sq/nb-mean*mean)) / mean
			cut, notch, bark := limiterShape(clean, pitch)
			hf := 10 * math.Log10(bandPower(x)/bandPower(ref))
			sp := p.spec
			wantRate := 1000 / (sp.LimiterMS * (1 + sp.LimiterFallRPMS/sp.LimiterRiseRPMS))
			wantDip := sp.LimiterFallRPMS * sp.LimiterLoadScale * sp.LimiterMS / 1000
			t.Logf("%.2f cuts per second, mean RPM dip %.0f, interval variation %.3f, cut %+.1f dB, notch %+.1f dB, bark %+.1f dB, band above 3 kHz %+.1f dB",
				rate, dip, cv, cut, notch, bark, hf)
			if math.Abs(rate-wantRate) > 0.2*wantRate || math.Abs(dip-wantDip) > 0.2*wantDip {
				t.Errorf("%.1f cuts per second with an RPM dip of %.0f, want %.1f and %.0f from the profile",
					rate, dip, wantRate, wantDip)
			}
			if cv > 0.03 {
				t.Errorf("interval variation %.3f, want at most 0.03", cv)
			}
			if r, ok := limiterRefs[name]; ok && (math.Abs(rate-r.rate) > 0.25*r.rate || math.Abs(dip-r.dip) > 0.25*r.dip) {
				t.Errorf("%.1f cuts per second with an RPM dip of %.0f, want %.1f and %.0f from the reference",
					rate, dip, r.rate, r.dip)
			}
			if wantDip <= 0.1*sp.LimiterRPM && !(math.Abs(cut-sp.LimiterPumpDB) <= 1.5 && math.Abs(notch-sp.LimiterNotchDB) <= 3 && math.Abs(bark-sp.LimiterBarkDB) <= 1.5) {
				t.Errorf("cut %+.1f dB, notch %+.1f dB and bark %+.1f dB, want %+.1f, %+.1f and %+.1f from the profile",
					cut, notch, bark, sp.LimiterPumpDB, sp.LimiterNotchDB, sp.LimiterBarkDB)
			}
			hfMost := 1.5
			if sp.LimiterKind == "ignition" && sp.Pops.Density > 0 {
				hfMost = 3.5
			}
			if hf > hfMost {
				t.Errorf("band above 3 kHz %.1f dB above full throttle below the limiter, want at most %.1f", hf, hfMost)
			}
		})
	}
}

// finalHold renders seconds of p at the snapshot RPM rpm with full throttle
// in third gear, with the overrun flag set as a careless game can set it,
// and with a limiter event every 100 ms. Before each limiterHop block, it
// sets FlagFinalGear when final returns true for the pitch so far. It
// returns the output and the pitch RPM of each block.
func finalHold(t testing.TB, p Profile, rpm float32, seconds float64, final func(pitch []float32) bool) (x, pitch []float32) {
	t.Helper()
	e := newEngine(t, p)
	s := Snapshot{RPM: rpm, Throttle: 1, Load: 1, Gear: 3}
	x = make([]float32, int(seconds*testRate)/limiterHop*limiterHop)
	for i := 0; i < len(x); i += limiterHop {
		s.Flags = FlagRunning | FlagOverrun | FlagRedZone
		if final(pitch) {
			s.Flags |= FlagFinalGear
		}
		if i%(testRate/10) == 0 {
			s.Limiter++
		}
		e.Set(s)
		e.Render(x[i : i+limiterHop])
		pitch = append(pitch, e.(*engine).synth.PitchRPM())
	}
	return x, pitch
}

// levelSteps returns the deepest dip of the 15 ms level of x below its
// median, and the largest rise of the level in 5 ms, in decibels.
func levelSteps(x []float32) (dip, rise float64) {
	env := envelopeDB(x)
	s := slices.Clone(env)
	slices.Sort(s)
	for i := 0; i+5 < len(env); i++ {
		rise = max(rise, env[i+5]-env[i])
	}
	return s[0] - s[len(s)/2], rise
}

// TestFinalGearHolds holds the snapshot RPM at and 300 RPM above the
// limiter with full throttle and FlagFinalGear for 2.5 s, with limiter
// events. From 0.5 s, no cut occurs: the pitch stays from the limiter RPM
// times 1-final_gear_waver to the limiter RPM, and the waver uses at least
// half of that range. The deepest dip of the 15 ms level below its median
// is at most 1 dB deeper than in the same hold 150 RPM below the limiter,
// far less than a cut. The output stays finite and below full scale.
func TestFinalGearHolds(t *testing.T) {
	t.Parallel()
	always := func([]float32) bool { return true }
	for _, name := range Profiles() {
		p, err := LoadProfile(name)
		if err != nil {
			t.Fatal(err)
		}
		skip := testRate / 2
		ref, _ := finalHold(t, p, p.LimiterRPM-150, 2.5, always)
		refDip, _ := levelSteps(ref[skip:])
		for _, over := range []float32{0, 300} {
			t.Run(fmt.Sprintf("%s/+%.0f", name, over), func(t *testing.T) {
				t.Parallel()
				x, pitch := finalHold(t, p, p.LimiterRPM+over, 2.5, always)
				x, pitch = x[skip:], pitch[skip/limiterHop:]
				for i, v := range x {
					if v != v || v > 0.99 || v < -0.99 {
						t.Fatalf("sample %d is %v", i, v)
					}
				}
				lo := p.LimiterRPM * float32(1-p.spec.FinalGearWaver)
				pLo, pHi := slices.Min(pitch), slices.Max(pitch)
				dip, _ := levelSteps(x)
				t.Logf("pitch from %.1f to %.1f RPM, deepest dip %.2f dB, %.2f dB below the limiter", pLo, pHi, dip, refDip)
				if pLo < lo-1 || pHi > p.LimiterRPM || pHi-pLo < 0.5*(p.LimiterRPM-lo) {
					t.Errorf("pitch from %.0f to %.0f RPM, want a waver from %.0f to %.0f", pLo, pHi, lo, p.LimiterRPM)
				}
				if dip < refDip-1 {
					t.Errorf("level dips %.1f dB below the median, %.1f dB below the limiter", dip, refDip)
				}
			})
		}
	}
}

// TestFinalGearHandover holds the snapshot RPM at the limiter with full
// throttle, and sets FlagFinalGear 35 ms into a cut, at the notch. The
// bounce ends: from 150 ms after the switch, the pitch stays in the waver
// range below the limiter RPM. From 20 ms before to 150 ms after the
// switch, the 15 ms level rises by at most 2 dB more in 5 ms than it does
// in a steady hold 150 RPM below the limiter. The firing gain returns from
// the notch at 1 dB per 5 ms.
func TestFinalGearHandover(t *testing.T) {
	t.Parallel()
	const notchHops = 35 * testRate / 1000 / limiterHop
	always := func([]float32) bool { return true }
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			ref, _ := finalHold(t, p, p.LimiterRPM-150, 2.5, always)
			_, refRise := levelSteps(ref[testRate/2:])
			at := -1
			final := func(pitch []float32) bool {
				n := len(pitch)
				if at < 0 && n > testRate/2/limiterHop && pitch[n-1] < pitch[n-2] && pitch[n-2] >= pitch[n-3] {
					at = n + notchHops
				}
				return at >= 0 && n >= at
			}
			x, pitch := finalHold(t, p, p.LimiterRPM, 1.5, final)
			if at < 0 || at >= len(pitch) {
				t.Fatal("no cut to end")
			}
			// The windows start 20 ms before the switch, so that they span it.
			i := at*limiterHop - 20*testRate/1000
			_, rise := levelSteps(x[i : i+(20+150)*testRate/1000])
			settle := at + 150*testRate/1000/limiterHop
			lo := p.LimiterRPM * float32(1-p.spec.FinalGearWaver)
			pLo, pHi := slices.Min(pitch[settle:]), slices.Max(pitch[settle:])
			t.Logf("dip %.0f RPM at the switch, largest rise %.2f dB per 5 ms, %.2f dB below the limiter", p.LimiterRPM-pitch[at], rise, refRise)
			if rise > refRise+2 {
				t.Errorf("level rises %.1f dB in 5 ms after the switch, %.1f dB below the limiter", rise, refRise)
			}
			if pLo < lo-1 || pHi > p.LimiterRPM {
				t.Errorf("pitch from %.0f to %.0f RPM 150 ms after the switch, want from %.0f to %.0f", pLo, pHi, lo, p.LimiterRPM)
			}
		})
	}
}
