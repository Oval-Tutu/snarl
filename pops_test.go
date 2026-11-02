package snarl

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/dsp"
	"github.com/oval-tutu/snarl/internal/scenario"
)

// withRequests adds one pop request at each of the times in seconds to
// the snapshots of set, with the strength rising from 0.1 by 0.3 per
// request.
func withRequests(set func(float64) Snapshot, at ...float64) func(float64) Snapshot {
	return func(t float64) Snapshot {
		s := set(t)
		for i, a := range at {
			if t >= a {
				s.PopRequest++
				s.PopStrength = min(1, 0.1+0.3*float32(i))
			}
		}
		return s
	}
}

// renderReports renders like render and returns the pop report after
// each block.
func renderReports(e Engine, seconds float64, set func(float64) Snapshot) ([]float32, []PopReport) {
	n := int(seconds * testRate)
	out := make([]float32, n)
	var reps []PopReport
	for i := 0; i < n; i += 256 {
		e.Set(set(float64(i) / testRate))
		e.Render(out[i:min(n, i+256)])
		reps = append(reps, e.Pops())
	}
	return out, reps
}

// quietProfile returns the profile with no own pops.
func quietProfile(t *testing.T, name string) Profile {
	t.Helper()
	p, err := LoadProfileWith(name, "pops.density=0")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPopReportMatchesRender checks that the report is consistent with
// the pops rendered. With the own pops off, a render with the first k of
// four requests differs from the render with k-1 from the block in which
// the report count reaches k or the next, and not before: the pop enters
// the synthesiser input at the control block that starts it, and the
// synthesiser renders that input one convolution block later. Each report
// names a requested pop of the kinds of the strength band.
func TestPopReportMatchesRender(t *testing.T) {
	t.Parallel()
	p := quietProfile(t, "v8-ls")
	times := []float64{0.5, 0.52, 1.5, 2.5}
	bands := [][2]PopKind{{KindPop, KindBurble}, {KindPop, KindBoom}, {KindBoom, KindCrackle}, {KindBoom, KindCrackle}}
	base := held(4000, 0.4)
	prev, _ := renderReports(newEngine(t, p), 3, base)
	for k := 1; k <= len(times); k++ {
		x, reps := renderReports(newEngine(t, p), 3, withRequests(base, times[:k]...))
		first := -1
		for i := range x {
			if x[i] != prev[i] {
				first = i / 256
				break
			}
		}
		reach := -1
		for b, r := range reps {
			if r.Count == uint32(k) {
				reach = b
				if r.Source != SourceRequest || (r.Kind != bands[k-1][0] && r.Kind != bands[k-1][1]) {
					t.Errorf("request %d: report %+v", k, r)
				}
				break
			}
		}
		if first < 0 || first < reach || first > reach+1 || reps[len(reps)-1].Count != uint32(k) {
			t.Errorf("request %d: output changes in block %d, report count %d in block %d, final count %d",
				k, first, k, reach, reps[len(reps)-1].Count)
		}
		prev = x
	}
}

// TestPopRequestBusy checks that every request plays and appears in the
// report while own pops play: 40 requests on a race i4-vtec that lifts off
// and coasts at a high density.
func TestPopRequestBusy(t *testing.T) {
	t.Parallel()
	p, err := LoadProfileWith("i4-vtec", "pops.density=4", `pops.character="race"`)
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(t, p)
	var at []float64
	for i := range 40 {
		at = append(at, 0.3+0.31*float64(i))
	}
	lift := func(t float64) Snapshot {
		s := Snapshot{RPM: 6000, Flags: FlagRunning | FlagOverrun}
		if int(t*10)%40 < 3 {
			s = Snapshot{RPM: 6000, Throttle: 1, Load: 1, Flags: FlagRunning}
		}
		return s
	}
	var requested, own uint32
	var last uint32
	_, reps := renderReports(e, 13, withRequests(lift, at...))
	for _, r := range reps {
		if r.Count != last {
			switch r.Source {
			case SourceRequest:
				requested++
			case SourceOwn:
				own++
			}
			last = r.Count
		}
	}
	// A block can start an own pop and a request, so the latest source
	// counts each block once. The total count still holds every request.
	if requested < 30 || own == 0 || last < 40+own {
		t.Errorf("%d pops: %d blocks with a requested pop, %d with an own pop", last, requested, own)
	}
}

// TestFlagNoPops checks that FlagNoPops stops the own pops of a lift-off
// and that a request still plays, and that a run-time race character
// over the profile makes more own pops than the stock character.
func TestFlagNoPops(t *testing.T) {
	t.Parallel()
	p, err := LoadProfileWith("i4-vtec", "pops.density=4")
	if err != nil {
		t.Fatal(err)
	}
	lifts := func(flags Flags, ch PopCharacter) func(float64) Snapshot {
		return func(t float64) Snapshot {
			s := Snapshot{RPM: 6000, Flags: FlagRunning | FlagOverrun | flags, PopCharacter: ch}
			if int(t*10)%40 < 5 {
				s.Throttle, s.Load, s.Flags = 1, 1, FlagRunning|flags
			}
			return s
		}
	}
	e := newEngine(t, p)
	_, reps := renderReports(e, 20, withRequests(lifts(FlagNoPops, CharacterProfile), 10))
	if r := reps[len(reps)-1]; r.Count != 1 || r.Source != SourceRequest {
		t.Errorf("FlagNoPops: report %+v, want one requested pop", r)
	}
	count := func(ch PopCharacter) uint32 {
		_, reps := renderReports(newEngine(t, p), 40, lifts(0, ch))
		return reps[len(reps)-1].Count
	}
	stock, race := count(CharacterStock), count(CharacterRace)
	if stock == 0 || race <= stock {
		t.Errorf("own pops in 40 s: stock %d, race %d", stock, race)
	}
}

// TestPopsConcurrent reads the report on another goroutine while Render
// runs with requests. Run it with -race. The count never falls.
func TestPopsConcurrent(t *testing.T) {
	e := newTestEngine(t, "v8-ls")
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		var last uint32
		for {
			select {
			case <-done:
				return
			default:
			}
			r := e.Pops()
			if r.Count < last || r.Strength < 0 || r.Strength > 1 {
				t.Errorf("report %+v after count %d", r, last)
				return
			}
			last = r.Count
		}
	})
	buf := make([]float32, 256)
	s := Snapshot{RPM: 5000, Throttle: 0.5, Load: 0.5, Flags: FlagRunning}
	for i := range 400 {
		if i%20 == 10 {
			s.PopRequest++
			s.PopStrength = float32(i%100) / 100
		}
		e.Set(s)
		e.Render(buf)
	}
	close(done)
	wg.Wait()
	if r := e.Pops(); r.Count != 20 {
		t.Errorf("%d pops from 20 requests", r.Count)
	}
}

// TestPopsFire checks that the own pops of the lift-off and the wind-down
// of the sweep play on every profile: the sweep renders with and without
// own pops, and the difference after the lift holds at least 1 per cent of
// the energy of the engine.
func TestPopsFire(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfileWith(name, "pops.density=1")
			if err != nil {
				t.Fatal(err)
			}
			off := sweep(p)
			on, reps := renderReports(newEngine(t, p), 12, sweep(p))
			quiet := render(newEngine(t, p), 12, func(t float64) Snapshot {
				s := off(t)
				s.Flags |= FlagNoPops
				return s
			})
			own := 0
			for i := 1; i < len(reps); i++ {
				if reps[i].Count != reps[i-1].Count && reps[i].Source == SourceOwn {
					own++
				}
			}
			var d, e float64
			for i := 7 * testRate; i < len(on); i++ {
				d += float64(on[i]-quiet[i]) * float64(on[i]-quiet[i])
				e += float64(quiet[i]) * float64(quiet[i])
			}
			if own < 2 || d < 0.01*e {
				t.Errorf("%d own pops, energy of the pops %.3g, of the engine %.3g", own, d, e)
			}
		})
	}
}

// popProfile returns the profile of popOverEngine: the pop density is 4,
// and only kind has a weight.
func popProfile(t *testing.T, name string, kind PopKind) Profile {
	t.Helper()
	w := [4]string{"0", "0", "0", "0"}
	w[kind-1] = "1"
	p, err := LoadProfileWith(name, "pops.density=4", "pops.weight_pop="+w[0], "pops.weight_boom="+w[1],
		"pops.weight_crackle="+w[2], "pops.weight_burble="+w[3])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// popSchedule gives the snapshots of popOverEngine block by block, from
// the random event times. full is true for an own pop schedule from 1.5 s
// before each event, when the throttle opens.
type popSchedule struct {
	rpm      float32
	request  bool
	strength float32
	cycle    float64
	at       float64
	rnd      dsp.Noise
	req      uint8
}

func newPopSchedule(p Profile, request bool, strength float32) popSchedule {
	c := popSchedule{rpm: p.RedlineRPM, request: request, strength: strength, cycle: 4.6, at: 2.5, rnd: dsp.NewNoise(7)}
	if request {
		c.cycle = 0.6
	}
	return c
}

// next returns the snapshot of block i. It must be called for each block
// in order.
func (c *popSchedule) next(i int) (s Snapshot, full bool) {
	t0 := float64(i*256) / testRate
	s = Snapshot{RPM: 0.7 * c.rpm, Throttle: 1, Load: 1, Flags: FlagRunning}
	if t0 >= c.at {
		c.at += c.cycle - 0.1 + 0.2*float64(c.rnd.Unipolar())
		c.req++
	}
	if c.request {
		s.PopRequest, s.PopStrength = c.req, c.strength
		return s, false
	}
	s.RPM = 0.85 * c.rpm
	if t0 >= c.at-1.5 {
		return s, true
	}
	s.Throttle, s.Load, s.Flags = 0, 0, FlagRunning|FlagOverrun
	return s, false
}

// popOverEngine renders n pops of one kind on the profile and returns the
// loudness of each in LU over the engine: the K-weighted level (ITU-R
// BS.1770) of the pop over the span from its start block that holds 90
// per cent of its energy, against the K-weighted level of the engine with
// no pop over the same span. The pop is the difference from a twin engine with no pops, after
// the leveller of both. Own pops come from lift-offs to closed throttle at
// 0.85 times the redline, every 4.5 to 4.7 s after 1.5 s of full
// throttle. Only the first pop of each lift plays, and the profile
// weights pick only the kind. The level of an own pop is brought to the
// median own level with its level in the report. Requested pops play
// every 0.5 to 0.7 s at full throttle at 0.7 times the redline, with a
// strength that gives the kind half of the time. The event times are
// random, so that a span does not start at the same engine phase each
// time.
func popOverEngine(t *testing.T, name string, kind PopKind, request bool, n int) []float64 {
	t.Helper()
	p := popProfile(t, name, kind)
	a, b := newEngine(t, p), newEngine(t, p)
	strength := map[PopKind]float32{KindPop: 0.5, KindBoom: 0.9, KindCrackle: 0.9, KindBurble: 0.1}[kind]
	sched := newPopSchedule(p, request, strength)
	var xa, xb []float32
	var starts []int
	var levels []float64
	var count uint32
	quiet, found := false, 0
	buf := make([]float32, 256)
	for i := 0; found <= n && i < 4000*testRate/256; i++ {
		s, full := sched.next(i)
		if full {
			quiet = false
		}
		if quiet {
			s.Flags |= FlagNoPops
		}
		a.Set(s)
		a.Render(buf)
		xa = append(xa, buf...)
		s.Flags |= FlagNoPops
		s.PopRequest = 0
		b.Set(s)
		b.Render(buf)
		xb = append(xb, buf...)
		if r := a.Pops(); r.Count != count {
			count = r.Count
			quiet = true
			// A pop of another kind ends the span of the pop before.
			starts = append(starts, i*256)
			levels = append(levels, -1)
			if r.Kind == kind {
				levels[len(levels)-1] = float64(r.Strength)
				found++
			}
		}
	}
	d, e := make([]float64, len(xa)), make([]float64, len(xa))
	for i := range xa {
		d[i], e[i] = float64(xa[i]-xb[i]), float64(xb[i])
	}
	kd, ke := analysis.KWeighted(d, testRate), analysis.KWeighted(e, testRate)
	var out []float64
	for j, st := range starts[:len(starts)-1] {
		if levels[j] < 0 {
			continue
		}
		end := min(starts[j+1], st+testRate)
		var tot float64
		for _, v := range d[st:end] {
			tot += v * v
		}
		var e9, kp, ks float64
		for i := st; i < end; i++ {
			e9 += d[i] * d[i]
			kp += kd[i] * kd[i]
			ks += ke[i] * ke[i]
			if e9 >= 0.9*tot && i >= st+testRate/100 {
				break
			}
		}
		db := 10 * math.Log10(kp/ks)
		if !request {
			db -= 20 * math.Log10(levels[j]/0.675)
		}
		out = append(out, db)
	}
	if len(out) < n {
		t.Fatalf("%s: %d pops of kind %s, want %d", name, len(out), kind, n)
	}
	return out[:n]
}

// popBands are the bands of TestPopLevelOverEngine: 3.5 LU around the
// loudness of the pop kinds over the engine, overLU plus popOffsetLU in
// internal/core/pops.go, -3, -4, -6.7 and -12.2 LU. The calibration sets
// the levels on single pops with no engine. In the engine, the pops of a
// kind vary in form, and an own pop sounds over the falling engine of a
// lift-off.
var popBands = map[PopKind][2]float64{KindPop: {-6.5, 0.5}, KindBoom: {-7.5, -0.5}, KindCrackle: {-10.2, -3.2}, KindBurble: {-15.7, -8.7}}

// TestPopLevelOverEngine checks the median loudness over the engine of each
// pop kind on each profile, for 41 own pops on overrun and 61 requested
// pops at full throttle, after the leveller: within 3.5 LU of the level of
// the kind (popBands). No profile stands out: each median is within 3.5 LU
// of the mean of the medians of the other profiles. It runs only outside
// short mode.
func TestPopLevelOverEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("the pop levels render about 4000 s of engine")
	}
	names := Profiles()
	var med [2][5][]float64
	var mu sync.Mutex
	t.Run("profiles", func(t *testing.T) {
		for _, name := range names {
			for r, request := range []bool{false, true} {
				for k := KindPop; k <= KindBurble; k++ {
					t.Run(fmt.Sprintf("%s/request-%t/%s", name, request, k), func(t *testing.T) {
						t.Parallel()
						n := 41
						if request {
							n = 61
						}
						v := popOverEngine(t, name, k, request, n)
						slices.Sort(v)
						m, b := v[len(v)/2], popBands[k]
						msg := fmt.Sprintf("%s, request %t, %s: median %.1f LU over the engine, want %.1f to %.1f", name, request, k, m, b[0], b[1])
						if m < b[0] || m > b[1] {
							t.Error(msg)
						} else {
							t.Log(msg)
						}
						mu.Lock()
						med[r][k] = append(med[r][k], m)
						mu.Unlock()
					})
				}
			}
		}
	})
	for r := range med {
		for k := KindPop; k <= KindBurble; k++ {
			if len(med[r][k]) != len(names) {
				continue
			}
			var sum float64
			for _, m := range med[r][k] {
				sum += m
			}
			for _, m := range med[r][k] {
				if d := m - (sum-m)/float64(len(names)-1); math.Abs(d) > 3.5 {
					t.Errorf("request %t, %s: a median of %.1f LU is %.1f LU from the other profiles", r == 1, k, m, d)
				}
			}
		}
	}
}

// levellerDip renders n requested pops of the kind on the profile at full
// throttle at 0.7 times the redline, 0.5 to 0.7 s apart, and a twin engine
// with no pops. It returns the medians over the pops of the deepest drop of
// the leveller gain below the twin in dB, and of the time in milliseconds
// that the gain stays more than 1 dB below the twin.
func levellerDip(t *testing.T, name string, kind PopKind, n int) (dipDB, ms float64) {
	t.Helper()
	p := popProfile(t, name, kind)
	a, b := newEngine(t, p), newEngine(t, p)
	strength := map[PopKind]float32{KindPop: 0.5, KindBoom: 0.9, KindCrackle: 0.9, KindBurble: 0.1}[kind]
	sched := newPopSchedule(p, true, strength)
	const hop = 32
	var ga, gb []float64
	var starts []int
	var count uint32
	buf := make([]float32, hop)
	for i := 0; len(starts) <= n; i++ {
		s, _ := sched.next(i)
		a.Set(s)
		s.PopRequest = 0
		b.Set(s)
		for range 256 / hop {
			a.Render(buf)
			b.Render(buf)
			ga = append(ga, float64(a.(*engine).synth.LevellerGain()))
			gb = append(gb, float64(b.(*engine).synth.LevellerGain()))
		}
		if r := a.Pops(); r.Count != count {
			count = r.Count
			starts = append(starts, len(ga))
		}
	}
	var dips, times []float64
	for j := range n {
		d, below := 0.0, 0
		for i := starts[j]; i < starts[j+1]; i++ {
			db := 20 * math.Log10(ga[i]/gb[i])
			d = min(d, db)
			if db < -1 {
				below++
			}
		}
		dips, times = append(dips, d), append(times, float64(below*hop)/testRate*1000)
	}
	slices.Sort(dips)
	slices.Sort(times)
	return dips[n/2], times[n/2]
}

// TestPopLeveller checks how the engine-sim leveller reacts to the pops:
// its gain drops by at most 0.1 dB against a twin engine with no pops, for
// requested pops of each kind on each profile. The pops join the
// synthesiser output after the leveller, so they take the gain that the
// engine sets and do not duck it. With the pops in the engine input, the
// gain dropped by 6 to 14 dB after each pop and stayed more than 1 dB down
// for 40 to 150 ms.
func TestPopLeveller(t *testing.T) {
	if testing.Short() {
		t.Skip("the leveller measure renders about 100 s of engine per case")
	}
	t.Parallel()
	for _, name := range Profiles() {
		for k := KindPop; k <= KindBurble; k++ {
			t.Run(fmt.Sprintf("%s/%s", name, k), func(t *testing.T) {
				t.Parallel()
				dip, ms := levellerDip(t, name, k, 21)
				msg := fmt.Sprintf("%s %s: the leveller gain drops by %.2f dB and stays more than 1 dB down for %.0f ms", name, k, -dip, ms)
				if dip < -0.1 {
					t.Error(msg)
				} else {
					t.Log(msg)
				}
			})
		}
	}
}

// TestPopPeak checks that the pops leave headroom: the overrun scenario of
// each profile, with race own pops at density 1 and requests of every
// strength, stays below full scale.
func TestPopPeak(t *testing.T) {
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfileWith(name, "pops.density=1", `pops.character="race"`)
			if err != nil {
				t.Fatal(err)
			}
			scn := parseUpstream(t, name, "overrun")
			var at []float64
			for i := range 30 {
				at = append(at, 0.3*float64(i+1))
			}
			snap := func(flags Flags) func(float64) Snapshot {
				return func(at float64) Snapshot {
					rpm, thr := scn.At(at)
					s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning | flags}
					if thr < 0.05 {
						s.Flags |= FlagOverrun
					}
					return s
				}
			}
			x := render(newEngine(t, p), scn.Duration(), withRequests(snap(0), at...))
			peak := checkBounded(t, x)
			engine := checkBounded(t, render(newEngine(t, p), scn.Duration(), snap(FlagNoPops)))
			msg := fmt.Sprintf("peak %.3f, with no pops %.3f", peak, engine)
			if peak >= 0.99 {
				t.Error(msg)
			} else {
				t.Log(msg)
			}
		})
	}
}

// overrunOwnPops renders the overrun scenario of the profile and returns
// the number of own pops.
func overrunOwnPops(t *testing.T, p Profile, scn *scenario.Scenario) int {
	t.Helper()
	e := newEngine(t, p)
	buf := make([]float32, 256)
	last, own := e.Pops(), 0
	for i := 0; i < int(scn.Duration()*testRate); i += 256 {
		rpm, thr := scn.At(float64(i) / testRate)
		s := Snapshot{RPM: float32(rpm), Throttle: float32(thr), Load: float32(thr), Flags: FlagRunning}
		if thr < 0.05 {
			s.Flags |= FlagOverrun
		}
		e.Set(s)
		e.Render(buf)
		if r := e.Pops(); r.Count != last.Count && r.Source == SourceOwn {
			own++
		}
		last = e.Pops()
	}
	return own
}

// TestOverrunOwnPops checks the own pops of the wind-down in the overrun
// scenario: a lift from full throttle at the top RPM, then closed
// throttle down to idle. Over 60 seeds, each profile has 1 to 4 own pops
// on average at the density of 0.15 of the profiles. A coast at a constant
// half of the redline for 20 s after a lift below the lift-off RPM has
// none. It runs only outside short mode.
func TestOverrunOwnPops(t *testing.T) {
	if testing.Short() {
		t.Skip("the own pops render 60 overrun scenarios per profile")
	}
	t.Parallel()
	for _, name := range Profiles() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatal(err)
			}
			scn := parseUpstream(t, name, "overrun")
			own := overrunOwnPops(t, p, scn)
			sum := 0
			for seed := range 60 {
				q, err := LoadProfileWith(name, fmt.Sprintf("seed=%d", 1000+seed))
				if err != nil {
					t.Fatal(err)
				}
				sum += overrunOwnPops(t, q, scn)
			}
			mean := float64(sum) / 60
			e := newEngine(t, p)
			_, reps := renderReports(e, 23, func(t float64) Snapshot {
				s := Snapshot{RPM: 0.5 * p.RedlineRPM, Throttle: 1, Load: 1, Flags: FlagRunning}
				if t >= 3 {
					s = Snapshot{RPM: 0.5 * p.RedlineRPM, Flags: FlagRunning | FlagOverrun}
				}
				return s
			})
			coast := 0
			for i := 1; i < len(reps); i++ {
				if reps[i].Count != reps[i-1].Count {
					coast++
				}
			}
			msg := fmt.Sprintf("%s: %d own pops with the profile seed, %.2f on average over 60 seeds, %d on a constant coast", name, own, mean, coast)
			if mean < 1 || mean > 4 || coast > 0 {
				t.Error(msg)
			} else {
				t.Log(msg)
			}
		})
	}
}
