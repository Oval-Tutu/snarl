package model

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl/internal/dsp"
)

// popStart is one pop start in a popRig run.
type popStart struct {
	at              float64 // Seconds from the start of the rig.
	kind, src, trig int
}

// popRig drives the pops alone, one control block of chunk samples at a
// time. It ages the pop events without their sound, so that long runs are
// fast.
type popRig struct {
	t   testing.TB
	p   *pops
	in  popInput
	n   int64 // Samples from the start.
	got []popStart
}

// newPopRig returns a rig with a warm exhaust and a chance multiplier of 1
// for the sport character. mod changes the spec first.
func newPopRig(t testing.TB, seed uint32, mod func(*Spec)) *popRig {
	t.Helper()
	s := DefaultSpec()
	s.Name = "p"
	s.IdleRPM, s.RedlineRPM = 800, 6800
	s.Pops.Density = 0.2
	if mod != nil {
		mod(&s)
	}
	p := newPops(&s, rate, chunk/float64(rate))
	p.noise = dsp.NewNoise(seed)
	p.heat = 1
	return &popRig{t: t, p: &p, in: popInput{rpm: 6000, heat: 1, level: 1, paths: 2, n: chunk}}
}

// run runs the rig for seconds. set changes the input before each block,
// with the time in seconds from the start of the run.
func (r *popRig) run(seconds float64, set func(t float64, in *popInput)) {
	r.t.Helper()
	for i := range int(seconds * rate / chunk) {
		if set != nil {
			set(float64(i*chunk)/rate, &r.in)
		}
		before := r.p.total
		r.p.control(&r.in)
		switch r.p.total - before {
		case 0:
		case 1:
			r.got = append(r.got, popStart{float64(r.n) / rate, r.p.lastKind, r.p.lastSrc, r.p.lastTrig})
		default:
			r.t.Fatalf("%d pops in one block", r.p.total-before)
		}
		for k := range r.p.ev {
			e := &r.p.ev[k]
			if e.left > 0 {
				e.left = max(0, e.left-chunk)
				if e.left == 0 {
					r.p.active--
				}
			}
		}
		r.n += chunk
	}
}

// from returns the pops of the source.
func (r *popRig) from(src int) []popStart {
	var out []popStart
	for _, g := range r.got {
		if g.src == src {
			out = append(out, g)
		}
	}
	return out
}

// lift runs a warm full-throttle pull of 0.5 s, then a sudden lift to
// closed throttle on overrun for seconds.
func (r *popRig) lift(seconds float64) {
	r.run(0.5, func(_ float64, in *popInput) { in.throttle, in.overrun = 1, false })
	r.run(seconds, func(_ float64, in *popInput) { in.throttle, in.overrun = 0, true })
}

// TestPopRequests checks that every request plays exactly one pop with the
// requested strength in the report, at any heat, cut or overrun state and
// with the own pops off. Requests at random times over 60 s each play once.
// With every slot busy with own pops, a request takes the slot of the
// oldest. With every slot busy with requested pops, it takes the oldest.
func TestPopRequests(t *testing.T) {
	r := newPopRig(t, 7, nil)
	r.p.heat = 0
	r.in.off, r.in.cut = true, true
	sent := 0
	u := dsp.NewNoise(99)
	next := 0.0
	r.run(60, func(t float64, in *popInput) {
		if t >= next {
			s := u.Unipolar()
			r.p.request(1, s)
			sent++
			next = t + 0.06 + 0.5*float64(u.Unipolar())
			if r.p.lastStrength != s && r.p.reqPending == 0 {
				r.t.Fatal("strength not reported")
			}
		}
		in.overrun = u.Unipolar() < 0.5
	})
	r.run(0.2, nil)
	if got := len(r.from(srcRequest)); got != sent || len(r.got) != sent || int(r.p.total) != sent {
		t.Errorf("%d requests, %d requested pops, %d pops, report count %d", sent, got, len(r.got), r.p.total)
	}
	for _, e := range r.p.ev {
		if e.left > 0 && e.amp <= 0 {
			t.Errorf("requested pop with gain %v on a cold exhaust", e.amp)
		}
	}

	r = newPopRig(t, 3, nil)
	for range maxPopEvents {
		r.p.own(&r.in, charSport, kindCrackle, trigLift, r.p.now)
		r.p.now += r.p.guard
	}
	if r.p.active != maxPopEvents {
		t.Fatalf("%d slots busy", r.p.active)
	}
	oldest := r.p.ev[0].start
	r.p.request(1, 0.9)
	r.p.now += r.p.guard
	r.run(float64(chunk)/rate, nil)
	if r.p.total != maxPopEvents+1 || r.p.lastSrc != srcRequest || r.p.active != maxPopEvents ||
		r.p.ev[0].src != srcRequest || r.p.ev[0].start == oldest {
		t.Errorf("request with busy slots: %d pops, source %d, %d active, slot 0 source %d", r.p.total, r.p.lastSrc, r.p.active, r.p.ev[0].src)
	}

	r = newPopRig(t, 4, nil)
	for range maxPopEvents + 1 {
		r.p.request(1, 0.5)
		r.run(0.06, nil)
	}
	if n := len(r.from(srcRequest)); n != maxPopEvents+1 || r.p.active != maxPopEvents {
		t.Errorf("%d requested pops with %d active, want %d with %d", n, r.p.active, maxPopEvents+1, maxPopEvents)
	}
}

// TestPopGuard checks that no pop starts within 50 ms of the last one and
// that a request inside the guard waits and plays, also behind an own pop.
// Up to four requests wait.
func TestPopGuard(t *testing.T) {
	r := newPopRig(t, 5, nil)
	r.p.request(3, 0.5)
	r.run(0.5, nil)
	if len(r.got) != 3 {
		t.Fatalf("%d pops from 3 requests", len(r.got))
	}
	for i := 1; i < len(r.got); i++ {
		if d := r.got[i].at - r.got[i-1].at; d < guardSec-1e-9 || d > guardSec+2.0*chunk/rate {
			t.Errorf("request %d plays %.1f ms after the one before", i+1, 1000*d)
		}
	}

	r = newPopRig(t, 6, nil)
	r.run(0.1, nil)
	r.p.own(&r.in, charSport, kindPop, trigLift, r.p.now)
	own := float64(r.p.now) / rate
	r.run(0.01, nil)
	r.p.request(1, 0.2)
	r.run(0.2, nil)
	if len(r.got) != 1 || r.got[0].at-own < guardSec-1e-9 || r.got[0].at-own > guardSec+2.0*chunk/rate {
		t.Errorf("request behind an own pop: %v, own pop at %.3f s", r.got, own)
	}

	r = newPopRig(t, 8, nil)
	r.p.request(6, 0.5)
	r.run(1, nil)
	if len(r.got) != maxPopEvents {
		t.Errorf("%d pops from 6 requests in one block, want %d", len(r.got), maxPopEvents)
	}
}

// TestPopStrengthBands checks that the strength picks the kinds: below 1/3
// a pop or a burble, below 2/3 a pop or a boom, and from 2/3 a boom or a
// short crackle. Both kinds of a band play, and no kind plays twice in a
// row.
func TestPopStrengthBands(t *testing.T) {
	for _, c := range []struct {
		strength float32
		a, b     int
	}{{0, kindPop, kindBurble}, {0.2, kindPop, kindBurble}, {1.0 / 3, kindPop, kindBoom}, {0.5, kindPop, kindBoom}, {2.0 / 3, kindBoom, kindCrackle}, {1, kindBoom, kindCrackle}} {
		r := newPopRig(t, 11, nil)
		var n [numKinds]int
		for i := range 200 {
			r.p.request(1, c.strength)
			r.run(0.06, nil)
			g := r.got[len(r.got)-1]
			if g.kind != c.a && g.kind != c.b {
				t.Fatalf("strength %.2f: kind %d", c.strength, g.kind)
			}
			if i > 0 && g.kind == r.got[len(r.got)-2].kind {
				t.Fatalf("strength %.2f: kind %d twice in a row", c.strength, g.kind)
			}
			if g.kind == kindCrackle {
				for _, e := range r.p.ev {
					if e.src == srcRequest && e.kind == kindCrackle && e.start == r.p.last && e.subs > shortCrackle {
						t.Fatalf("requested crackle with %d sub-events", e.subs)
					}
				}
			}
			n[g.kind]++
		}
		if n[c.a] == 0 || n[c.b] == 0 {
			t.Errorf("strength %.2f: kinds %v", c.strength, n)
		}
	}
}

// liftStats runs the given number of lifts, each with its own seed, and returns the share of
// lifts with 1, 2 and 3 own pops, and the first and later kinds counted.
func liftStats(t *testing.T, lifts int, mod func(*Spec), set func(in *popInput)) (share [liftMaxPops + 1]float64, first, later [numKinds]int) {
	t.Helper()
	for i := range lifts {
		r := newPopRig(t, uint32(1000+i), mod)
		if set != nil {
			set(&r.in)
		}
		r.lift(2.4)
		own := r.from(srcOwn)
		if len(own) > liftMaxPops {
			t.Fatalf("lift %d: %d pops", i, len(own))
		}
		share[len(own)]++
		for j, g := range own {
			if j == 0 {
				first[g.kind]++
				if d := g.at - 0.5; d < liftDelayMin || d > liftDelayMax+float64(chunk)/rate {
					t.Fatalf("lift %d: first pop %.0f ms after the lift", i, 1000*d)
				}
				continue
			}
			later[g.kind]++
			if d := g.at - own[j-1].at; d < runGapMin || d > runGapMax+float64(chunk)/rate {
				t.Fatalf("lift %d: pop interval %.0f ms", i, 1000*d)
			}
		}
	}
	for k := range share {
		share[k] /= float64(lifts)
	}
	return share, first, later
}

// TestPopLiftOff checks the lift-off statistics over 4000 seeded lifts:
// the chance of a run, the run lengths from the chance decay, the delays,
// the intervals and the kinds of the sport character.
func TestPopLiftOff(t *testing.T) {
	share, first, later := liftStats(t, 4000, nil, nil)
	c := liftChance
	want := [4]float64{1 - c, c * (1 - c*liftDecay), c * c * liftDecay * (1 - c*liftDecay*liftDecay), c * c * c * liftDecay * liftDecay * liftDecay}
	for k := range share {
		if math.Abs(share[k]-want[k]) > 0.025 {
			t.Errorf("runs of %d pops: share %.3f, want %.3f", k, share[k], want[k])
		}
	}
	t.Logf("run length shares 0 to 3: %.3f, want %.3f", share, want)
	if first[kindPop]+first[kindBurble] > 0 || later[kindBoom]+later[kindCrackle] > 0 || first[kindBoom]*first[kindCrackle] == 0 {
		t.Errorf("first kinds %v, later kinds %v", first, later)
	}
}

// TestPopSlowLift checks that a lift over 1 s, and a lift at low RPM or
// on a cold exhaust, give no pop, even with a chance of 1.
func TestPopSlowLift(t *testing.T) {
	race := func(s *Spec) { s.Pops.Character = "race" }
	for i := range 300 {
		r := newPopRig(t, uint32(i), race)
		r.run(0.5, func(_ float64, in *popInput) { in.throttle = 1 })
		r.run(2.4, func(t float64, in *popInput) {
			in.throttle = float32(max(0, 1-t))
			in.overrun = in.throttle < 0.05
		})
		cold := newPopRig(t, uint32(i), race)
		cold.p.heat, cold.in.heat = 0, 0
		cold.lift(2.4)
		low := newPopRig(t, uint32(i), race)
		low.in.rpm = 0.55 * 6800
		low.lift(2.4)
		if len(r.got)+len(cold.got)+len(low.got) > 0 {
			t.Fatalf("seed %d: slow lift %v, cold %v, low RPM %v", i, r.got, cold.got, low.got)
		}
	}
}

// TestPopWindDown checks the wind-down bands on a closed-throttle sweep
// from 6000 RPM to idle with a chance of 1. The rig has a redline of 6800
// and an idle of 800 RPM, so 3 bands split 4080 down to 1040 RPM. Each
// band plays one pop or burble as the RPM falls into it, with more pops at
// high RPM, and nothing plays below 1040 RPM. A coast at a constant RPM and
// an RPM that wavers by less than the hysteresis around a band top play
// nothing.
func TestPopWindDown(t *testing.T) {
	hard := func(s *Spec) { s.Pops.Density = 1 }
	var kinds [3][numKinds]int
	for i := range 400 {
		r := newPopRig(t, uint32(100+i), hard)
		r.run(8, func(t float64, in *popInput) {
			in.overrun = true
			in.rpm = float32(max(800, 6000-700*t))
		})
		own := r.from(srcOwn)
		if len(own) != 3 {
			t.Fatalf("seed %d: %d wind-down pops, want 3: %v", i, len(own), own)
		}
		for j, g := range own {
			rpm := 6000 - 700*g.at
			top := 4080 - float64(j)*(4080-1040)/3
			if g.trig != trigWind || rpm > top || rpm < top-80 || (g.kind != kindPop && g.kind != kindBurble) {
				t.Fatalf("seed %d: pop %d %+v at %.0f RPM, band top %.0f RPM", i, j, g, rpm, top)
			}
			kinds[j][g.kind]++
		}
	}
	for j, k := range kinds {
		want := 0.8 - 0.3*float64(j)
		if got := float64(k[kindPop]) / 400; math.Abs(got-want) > 0.06 {
			t.Errorf("band %d: pop share %.2f, want %.2f", j+1, got, want)
		}
	}
	steady := newPopRig(t, 22, hard)
	steady.in.rpm = 3000
	steady.run(0.1, nil)
	steady.run(120, func(_ float64, in *popInput) { in.overrun = true })
	waver := newPopRig(t, 23, hard)
	waver.in.rpm = 3160
	waver.run(0.1, nil)
	waver.run(120, func(t float64, in *popInput) {
		in.overrun = true
		in.rpm = float32(3060 + 100*math.Cos(t))
	})
	if n, m := len(steady.got), len(waver.got); n > 0 || m > 1 {
		t.Errorf("constant RPM: %d pops, RPM wavering around a band top: %d pops, want none and one", n, m)
	}
}

// TestPopLimits checks the limits of the own pops with lift-offs and
// wind-downs at a chance of 1: at most 6 in any 10 s, and at least 0.7 s
// between pops of two triggers. The 6 per 10 s limit is reached.
func TestPopLimits(t *testing.T) {
	r := newPopRig(t, 31, func(s *Spec) { s.Pops.Density, s.Pops.Character = 4, "race" })
	for range 60 {
		r.run(0.2, func(_ float64, in *popInput) { in.rpm, in.throttle, in.overrun = 6000, 1, false })
		r.run(3.6, func(t float64, in *popInput) {
			in.rpm, in.throttle, in.overrun = float32(6000-1200*t), 0, true
		})
	}
	own := r.from(srcOwn)
	most, mixed := 0, 0
	for i, g := range own {
		n := 0
		for _, h := range own[i:] {
			if h.at-g.at < ownSeconds-1e-9 {
				n++
			}
		}
		most = max(most, n)
		if i > 0 && g.trig != own[i-1].trig {
			mixed++
			if d := g.at - own[i-1].at; d < gapSec-1e-9 {
				t.Errorf("pops of two triggers %.2f s apart", d)
			}
		}
	}
	if most != ownWindow || mixed == 0 {
		t.Errorf("at most %d pops in 10 s, want %d, and %d trigger changes", most, ownWindow, mixed)
	}
	t.Logf("%d own pops in %.0f s, at most %d in 10 s", len(own), float64(r.n)/rate, most)
}

// TestPopsOff checks that the off input stops the own pops of lift-offs
// and wind-downs, and that requests still play.
func TestPopsOff(t *testing.T) {
	r := newPopRig(t, 41, func(s *Spec) { s.Pops.Density, s.Pops.Character = 4, "race" })
	r.in.off = true
	for range 10 {
		r.in.rpm = 6000
		r.run(0.5, func(_ float64, in *popInput) { in.throttle, in.overrun = 1, false })
		r.run(3.5, func(t float64, in *popInput) {
			in.rpm, in.throttle, in.overrun = float32(6000-1500*t), 0, true
		})
		r.p.request(1, 0.5)
	}
	r.run(0.1, nil)
	if len(r.from(srcOwn)) > 0 || len(r.from(srcRequest)) != 10 {
		t.Errorf("own pops off: %d own pops, %d requested pops", len(r.from(srcOwn)), len(r.from(srcRequest)))
	}
}

// TestPopCharacters checks each character: the lift-off chance over 2000
// lifts, the kinds of the first and later pops, the gain of an own pop and
// of a requested pop, and the run-time character over the profile.
func TestPopCharacters(t *testing.T) {
	gain := func(ch int, request bool) float32 {
		r := newPopRig(t, 51, nil)
		if request {
			r.in.character = ch + 1
			r.p.request(1, 0.5)
			r.run(float64(chunk)/rate, nil)
		} else {
			r.p.own(&r.in, ch, kindPop, trigLift, 0)
		}
		e := &r.p.ev[0]
		return e.amp / r.p.kindGain[e.path][b2i(e.src == srcRequest)][e.kind]
	}
	for ch, c := range characters {
		name := characterNames[ch]
		share, first, later := liftStats(t, 2000, func(s *Spec) { s.Pops.Character = name }, nil)
		if want := min(1, liftChance*c.density); math.Abs(1-share[0]-want) > 0.035 {
			t.Errorf("%s: lift-off chance %.3f, want %.3f", name, 1-share[0], want)
		}
		soft := first[kindPop] + first[kindBurble]
		hard := later[kindBoom] + later[kindCrackle]
		if (soft > 0) != (ch == charStock) || (hard > 0) != (ch == charRace) {
			t.Errorf("%s: first kinds %v, later kinds %v", name, first, later)
		}
		for _, g := range []struct {
			request bool
			db      float64
		}{{false, c.gainDB}, {true, c.requestDB}} {
			if got := 20 * math.Log10(float64(gain(ch, g.request)/gain(charSport, g.request))); math.Abs(got-g.db) > 0.01 {
				t.Errorf("%s: gain %.2f dB, want %.2f dB (request %t)", name, got, g.db, g.request)
			}
		}
	}
	share, _, _ := liftStats(t, 1000, func(s *Spec) { s.Pops.Character = "stock" }, func(in *popInput) { in.character = charRace + 1 })
	if share[0] > 0.01 {
		t.Errorf("run-time race character over a stock profile: %.3f of lifts with no pop", share[0])
	}
}

// TestPopMix checks that the run kind picks follow the profile weights
// within 1.5 percentage points over 20000 picks, and that no kind with
// weight 0 plays.
func TestPopMix(t *testing.T) {
	for _, w := range [][numKinds]float64{{0.1, 0.2, 0.3, 0.4}, {0.5, 0, 0.5, 0}} {
		s := DefaultSpec()
		s.Pops.WeightPop, s.Pops.WeightBoom, s.Pops.WeightCrackle, s.Pops.WeightBurble = w[0], w[1], w[2], w[3]
		p := newPops(&s, rate, chunk/float64(rate))
		var picks [numKinds]int
		for range 20000 {
			picks[pick(p.noise.Unipolar(), &p.cum[charStock][0])]++
		}
		for k := range numKinds {
			if got := float64(picks[k]) / 20000; math.Abs(got-w[k]) > 0.015 || (w[k] == 0 && picks[k] > 0) {
				t.Errorf("weights %v: kind %d share %.3f", w, k, got)
			}
		}
	}
	// A group with no weight falls back to every kind with a weight.
	cum := cumulative([numKinds]float64{0.5, 0, 0, 0.5}, characters[charSport].first)
	if cum != [numKinds]float32{0.5, 0.5, 0.5, 1} {
		t.Errorf("fallback weights %v", cum)
	}
}

// TestPopCap checks that at most maxPopEvents pops sound at once, that an
// own pop or a bang with every slot busy does not start, and that the path
// jumps return to zero when the last pop ends.
func TestPopCap(t *testing.T) {
	s := DefaultSpec()
	p := newPops(&s, rate, chunk/float64(rate))
	p.heat = 1
	for i := range maxPopEvents + 3 {
		e := p.fire(2, kindCrackle, srcOwn, 1, 1, 1)
		if (e == nil) != (i >= maxPopEvents) {
			t.Fatalf("pop %d: started %v with %d active", i, e != nil, p.active)
		}
	}
	if p.active != maxPopEvents || p.total != maxPopEvents {
		t.Fatalf("%d active and %d started, want %d", p.active, p.total, maxPopEvents)
	}
	p.bang(2)
	if p.total != maxPopEvents {
		t.Error("a bang started with every slot busy")
	}
	for range 2 * rate {
		p.tick()
		if p.active > maxPopEvents {
			t.Fatalf("%d active", p.active)
		}
	}
	if p.active != 0 || p.jump != [MaxBanks]float32{} {
		t.Errorf("%d active after 2 s, jumps %v", p.active, p.jump)
	}
}

// TestPopBangWithPopsOff checks that ignition cut bangs play with the own
// pops off.
func TestPopBangWithPopsOff(t *testing.T) {
	s := limiterSpec("ignition")
	s.Pops.Density = 1
	y, err := New(&s, rate)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 256)
	c := Control{RPM: float32(s.LimiterRPM), Throttle: 1, Load: 1, Running: true, Gear: 1, NoPops: true}
	for range rate / 256 {
		y.Render(buf, c)
	}
	n, _, src, _ := y.PopReport()
	if n == 0 || src != srcLimiter {
		t.Errorf("%d pops at the limiter with the own pops off, last source %d", n, src)
	}
}

// armed reports whether an own pop can start: a lift-off run or a coast
// is in progress.
func (p *pops) armed() bool {
	return p.runLeft > 0
}
