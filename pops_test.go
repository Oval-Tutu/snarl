package snarl

import (
	"sync"
	"testing"
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

// quietProfile returns the profile with no own pops and no bangs.
func quietProfile(t *testing.T, name string) Profile {
	t.Helper()
	p, err := LoadProfileWith(name, "pops.density=0")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPopReportMatchesRender checks that the report is consistent with
// the pops rendered. With the own pops and the bangs off, a render with
// the first k of four requests differs from the render with k-1 from the
// block in which the report count reaches k, and not before. Each report
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
		if first < 0 || first != reach || reps[len(reps)-1].Count != uint32(k) {
			t.Errorf("request %d: output changes in block %d, report count %d in block %d, final count %d",
				k, first, k, reach, reps[len(reps)-1].Count)
		}
		prev = x
	}
}

// TestPopRequestBusy checks that every request plays and appears in the
// report while own pops play: 40 requests on a race i4-vtec that lifts off
// and coasts at a high density. The model tests cover four busy slots.
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
// over the profile makes own pops where the stock character makes fewer.
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
