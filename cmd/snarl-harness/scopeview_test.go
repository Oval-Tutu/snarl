package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/scope"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/stream"
)

func TestCylinderCounts(t *testing.T) {
	ps, _, _, err := loadProfiles("v8-ls", findCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	n := cylinderCounts(ps, findCatalogue())
	want := map[string]int{"i4-vtec": 4, "v8-ls": 8, "f4-ej25": 4, "v12-412": 12, "v12-merlin": 12}
	for i, p := range ps {
		if w, ok := want[p.Name]; ok && n[i] != w {
			t.Errorf("%s: %d cylinders, want %d", p.Name, n[i], w)
		}
		if n[i] == 0 {
			t.Errorf("%s: no cylinder count", p.Name)
		}
	}
	if n := cylinderCounts(ps, ""); n[0] != 0 {
		t.Errorf("no catalogue: %d cylinders, want 0", n[0])
	}
}

func TestFiringHz(t *testing.T) {
	t.Parallel()
	if f := firingHz(3000, 8); f != 200 {
		t.Errorf("V8 at 3000 RPM fires at %v Hz, want 200", f)
	}
}

// countTap counts the blocks it receives.
type countTap struct{ n int }

func (c *countTap) Capture([]float32) { c.n++ }

func TestReadWithTeeDoesNotAllocate(t *testing.T) {
	p, err := snarl.LoadProfile("v12-merlin")
	if err != nil {
		t.Fatal(err)
	}
	e, err := snarl.New(p, stream.SampleRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	src := stream.New()
	src.SetEngine(e)
	sc, c := scope.New(stream.SampleRate, stream.Block), &countTap{}
	src.SetTap(tee{sc, c})
	buf := make([]byte, stream.BlockBytes)
	_, _ = src.Read(buf)
	if a := testing.AllocsPerRun(100, func() { _, _ = src.Read(buf) }); a != 0 {
		t.Errorf("%v allocations per Read with the tee, want 0", a)
	}
	if c.n != 102 {
		t.Errorf("second tap got %d blocks, want 102", c.n)
	}
}

// BenchmarkDrawScope measures the draw cost of the meter and the spectrum.
func BenchmarkDrawScope(b *testing.B) {
	ps, i, _, err := loadProfiles("v8-ls", "")
	if err != nil {
		b.Fatal(err)
	}
	h := &harness{profiles: ps, profile: i, scope: scope.New(stream.SampleRate, stream.Block), cylinders: []int{4, 8, 4, 12, 12}}
	h.snap.RPM = 3000
	screen := ebiten.NewImage(screenW, screenH)
	for b.Loop() {
		h.drawScope(screen, 0)
	}
}
