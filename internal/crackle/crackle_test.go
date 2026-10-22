package crackle

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"
)

const rate = 44100

func peak(y []float32) float64 {
	var p float64
	for _, v := range y {
		p = max(p, math.Abs(float64(v)))
	}
	return p
}

func TestParse(t *testing.T) {
	p := Default()
	q, err := Parse(p.Marshal("header"))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := string(p.Marshal("")), string(q.Marshal("")); a != b {
		t.Errorf("the parameter file changes in a round trip:\n%s\n%s", a, b)
	}
	data := string(p.Marshal(""))
	for _, c := range []struct{ name, data string }{
		{"unknown key", data + "nope = 1\n"},
		{"key again", data + "attack_ms = 1\n"},
		{"missing key", strings.Replace(data, "attack_ms", "# attack_ms", 1)},
		{"wrong count", strings.Replace(data, "place_db = [", "place_db = [1, ", 1)},
		{"bad number", strings.Replace(data, "attack_ms = ", "attack_ms = x", 1)},
	} {
		if _, err := Parse([]byte(c.data)); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}
}

// TestSources checks that crackle.sources.toml has one entry of a known
// kind for each key of crackle.toml, and no other entry.
func TestSources(t *testing.T) {
	data, err := os.ReadFile("crackle.sources.toml")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{"measured": true, "fitted": true, "snarl": true}
	seen := map[string]int{}
	var key string
	for _, line := range strings.Split(string(data), "\n") {
		name, val, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		val = strings.Trim(val, `"`)
		switch name {
		case "key":
			key = val
			seen[key]++
		case "kind":
			if !kinds[val] {
				t.Errorf("%s has the kind %q", key, val)
			}
		case "from":
			if val == "" {
				t.Errorf("%s has no source", key)
			}
		}
	}
	for _, k := range Default().Keys() {
		if seen[k] != 1 {
			t.Errorf("%s has %d entries, want 1", k, seen[k])
		}
		delete(seen, k)
	}
	for k := range seen {
		t.Errorf("%s is not a key of crackle.toml", k)
	}
}

func TestDeterministic(t *testing.T) {
	p := Default()
	a, b := Render(p, rate, 3, Options{}), Render(p, rate, 3, Options{})
	if !slices.Equal(a, b) {
		t.Error("two renders of one seed differ")
	}
	if c := Render(p, rate, 4, Options{}); slices.Equal(a, c) {
		t.Error("two seeds give the same render")
	}
	for _, v := range a {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatal("the render is not finite")
		}
	}
}

func TestNoAllocations(t *testing.T) {
	v := New(Default(), rate, 1)
	buf := make([]float32, 256)
	allocs := testing.AllocsPerRun(200, func() {
		if !v.Active() {
			v.Trigger(Options{})
		}
		v.Process(buf)
	})
	if allocs != 0 {
		t.Errorf("%.1f allocations per block, want 0", allocs)
	}
}

func TestLengthControl(t *testing.T) {
	p := Default()
	for _, c := range []struct {
		o    Options
		want int
	}{
		{Options{Shocks: 1}, p.ShocksMin},
		{Options{Shocks: 1000}, p.ShocksMax},
		{Options{Shocks: p.ShocksMin + 1}, p.ShocksMin + 1},
		{Options{Single: true}, 1},
	} {
		v := New(p, rate, 2)
		v.Trigger(c.o)
		buf := make([]float32, 256)
		for v.Active() {
			v.Process(buf)
		}
		if v.count != c.want {
			t.Errorf("%+v: %d shocks, want %d", c.o, v.count, c.want)
		}
	}
	// The fade ends a train within this time after the span of its shocks.
	fade := p.FadeDelayMS + 100/p.FadeDBPerMS + 50
	for _, l := range []float64{1, 1e6} {
		y := Render(p, rate, 5, Options{LengthMS: l})
		got := float64(len(y)) / rate * 1000
		lo, hi := p.LengthMS[0], p.LengthMS[4]
		if got > hi-p.LengthOffsetMS+fade || (l > hi && got < lo) {
			t.Errorf("length %g ms gives %.0f ms, want inside the range %g to %g ms and the fade", l, got, lo, hi)
		}
	}
}

func TestLevel(t *testing.T) {
	p := Default()
	a := Render(p, rate, 6, Options{})
	b := Render(p, rate, 6, Options{LevelDB: 6})
	if r := 20 * math.Log10(peak(b)/peak(a)); math.Abs(r-6) > 0.01 {
		t.Errorf("LevelDB 6 changes the peak by %.3f dB", r)
	}
}

func TestRate(t *testing.T) {
	p := Default()
	a := Render(p, rate, 8, Options{Shocks: 40})
	b := Render(p, 2*rate, 8, Options{Shocks: 40})
	if r := float64(len(b)) / float64(len(a)); r < 1.5 || r > 2.5 {
		t.Errorf("twice the rate gives %.2f times the samples", r)
	}
	if peak(b) == 0 {
		t.Error("the render at twice the rate is quiet")
	}
}

// BenchmarkVoice renders one second of trains at 44.1 kHz in blocks of
// 256 samples, with a new train when one ends. The time per operation is
// the time per second of audio.
func BenchmarkVoice(b *testing.B) {
	v := New(Default(), rate, 1)
	buf := make([]float32, 256)
	for b.Loop() {
		for range rate / len(buf) {
			if !v.Active() {
				v.Trigger(Options{})
			}
			clear(buf)
			v.Process(buf)
		}
	}
}
