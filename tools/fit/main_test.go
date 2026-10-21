package main

import "testing"

// TestChooseN checks that the smallest N from minN within the tolerance
// wins, and that the lowest error wins when no N is within it.
func TestChooseN(t *testing.T) {
	errs := []float64{3, 2.5, 1.4, 1.1, 0.9, 0.8}
	if n, ok := chooseN(errs, 2, 1); n != 5 || !ok {
		t.Errorf("chooseN = %d, %v, want 5, true", n, ok)
	}
	if n, ok := chooseN(errs, 2, 0.5); n != 6 || ok {
		t.Errorf("chooseN = %d, %v, want 6, false", n, ok)
	}
}
