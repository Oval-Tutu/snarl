package esynth

import (
	"path/filepath"
	"testing"
)

// The excerpts are engine-sim-render --dump-synth records of the i4-vtec
// engine (atg-video-1/05_honda_vtec.mr): --settle 0.6 --hold 3000:0.5:0.2
// --volume 0.5, with and without --no-noise.
var excerpts = []string{"i4-vtec-noise", "i4-vtec-quiet"}

func loadExcerpt(t *testing.T, name string) (*Dump, [][]int16, []int16) {
	t.Helper()
	d, err := LoadDump(filepath.Join("..", "..", "testdata", "esynth", name))
	if err != nil {
		t.Fatal(err)
	}
	irs, err := d.ImpulseResponses("")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := d.ReadOutput()
	if err != nil {
		t.Fatal(err)
	}
	return d, irs, ref
}

// TestNullFloat32 checks that the port in Float32 precision reproduces the
// engine-sim synthesiser output bit for bit, the random parts included.
func TestNullFloat32(t *testing.T) {
	for _, name := range excerpts {
		t.Run(name, func(t *testing.T) {
			d, irs, ref := loadExcerpt(t, name)
			port, err := Render(d, irs, Float32)
			if err != nil {
				t.Fatal(err)
			}
			diff, err := Compare(port, ref)
			if err != nil {
				t.Fatal(err)
			}
			if diff.Differing != 0 {
				t.Errorf("%s", diff)
			}
		})
	}
}

// TestNullFloat64 checks that the port in Float64 precision stays within
// the stated tolerance of engine-sim: a peak difference of at most 6
// 16-bit steps and an RMS difference of at most 1 step.
func TestNullFloat64(t *testing.T) {
	for _, name := range excerpts {
		t.Run(name, func(t *testing.T) {
			d, irs, ref := loadExcerpt(t, name)
			port, err := Render(d, irs, Float64)
			if err != nil {
				t.Fatal(err)
			}
			diff, err := Compare(port, ref)
			if err != nil {
				t.Fatal(err)
			}
			if diff.PeakDiff > 6 || diff.RMSDiff > 1 || diff.RMSSignal < 1000 {
				t.Errorf("%s, RMS signal %.0f", diff, diff.RMSSignal)
			}
		})
	}
}

// TestRandom checks the first glibc rand() values with the default seed.
func TestRandom(t *testing.T) {
	g := NewRandom(1)
	for i, want := range []int32{1804289383, 846930886, 1681692777, 1714636915, 1957747793} {
		if got := g.Rand(); got != want {
			t.Errorf("rand() %d = %d, want %d", i, got, want)
		}
	}
}

// TestMinstdRand0 checks std::minstd_rand0: its 10000th value is 1043618065,
// as the C++ standard requires.
func TestMinstdRand0(t *testing.T) {
	g := newMinstdRand0()
	var v uint64
	for range 10000 {
		v = g.next()
	}
	if v != 1043618065 {
		t.Errorf("10000th value %d, want 1043618065", v)
	}
}

// TestUniformFloat32 checks the libstdc++ 16 uniform_real_distribution
// draws: the canonical value is (x - 1) / 127 / 2^24, redrawn at 1 or above.
func TestUniformFloat32(t *testing.T) {
	g := newMinstdRand0()
	if got, want := g.uniformFloat32(0, 9), float32(16806/127)/(1<<24)*9; got != want {
		t.Errorf("first draw %g, want %g", got, want)
	}
	// Engine values from 127 * 2^24 + 1 give a canonical value of 1 or
	// above, which the generator redraws.
	for range 100000 {
		if v := g.uniformFloat32(0, 9); v < 0 || v >= 9 {
			t.Fatalf("draw %g out of [0, 9)", v)
		}
	}
}

// TestRoundedTan checks the correctly rounded tan and cos against values
// that math.Tan and math.Cos also give for these arguments.
func TestRoundedTan(t *testing.T) {
	if got := roundedTan(0, Float64); got != 0 {
		t.Errorf("tan(0) = %g", got)
	}
	if got := roundedTan(0.5, Float64); got != 0.5463024898437905 {
		t.Errorf("tan(0.5) = %.17g", got)
	}
	if got := roundedCos(1); got != 0.5403023058681398 {
		t.Errorf("cos(1) = %.17g", got)
	}
}

// TestRingBufferFull checks the engine-sim behaviour that a buffer filled
// to its capacity reports size 0, which the synthesiser output buffer
// relies on at start.
func TestRingBufferFull(t *testing.T) {
	r := newRingBuffer[int16](4)
	for range 4 {
		r.write(1)
	}
	if r.size() != 0 {
		t.Errorf("size %d, want 0", r.size())
	}
	r.write(2)
	got := make([]int16, 1)
	r.readAndRemove(got)
	if got[0] != 2 || r.size() != 0 {
		t.Errorf("read %d with size %d, want 2 with size 0", got[0], r.size())
	}
}
