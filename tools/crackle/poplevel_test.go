package main

import "testing"

// TestSpan90 checks the span that holds 90 per cent of the energy: the
// sample that reaches it, the least length, and no span with no energy.
func TestSpan90(t *testing.T) {
	x := []float64{3, 1, 1, 0, 0, 0}
	// The energy is 11. The first two samples hold 10 of it, over 9.9.
	if n := span90(x, 1); n != 2 {
		t.Errorf("span %d, want 2", n)
	}
	if n := span90(x, 4); n != 4 {
		t.Errorf("span %d with a least length of 4, want 4", n)
	}
	if n := span90(x, 10); n != len(x) {
		t.Errorf("span %d with a least length past the end, want %d", n, len(x))
	}
	if n := span90(make([]float64, 4), 1); n != 0 {
		t.Errorf("span %d of silence, want 0", n)
	}
}

// TestParseCases checks the PROFILE=SCENARIO arguments.
func TestParseCases(t *testing.T) {
	cs, err := parseCases([]string{"v8-ls=a.scn", "i4-vtec=b.scn"})
	if err != nil || len(cs) != 2 || cs[1] != (levelCase{"i4-vtec", "b.scn"}) {
		t.Errorf("cases %v, error %v", cs, err)
	}
	for _, a := range []string{"v8-ls", "=a.scn", "v8-ls="} {
		if _, err := parseCases([]string{a}); err == nil {
			t.Errorf("%q: no error", a)
		}
	}
}

// TestFaded checks the gain and the fades at both ends.
func TestFaded(t *testing.T) {
	x := []float64{1, 1, 1, 1, 1, 1}
	y := faded(x, 20, 2)
	want := []float32{0, 5, 10, 10, 5, 0}
	for i := range y {
		if d := y[i] - want[i]; d > 1e-5 || d < -1e-5 {
			t.Errorf("sample %d: %g, want %g", i, y[i], want[i])
		}
	}
}
