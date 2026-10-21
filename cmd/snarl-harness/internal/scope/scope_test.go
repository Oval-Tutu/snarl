package scope

import (
	"math"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/stream"
)

const testRate, testBlock = 48000, 256

// feed captures seconds of a sine of amplitude a at f Hz and updates s
// after every three blocks, as a game tick does.
func feed(s *Scope, f, a, seconds float64) {
	block := make([]float32, testBlock)
	blocks := int(seconds * testRate / testBlock)
	for i := range blocks {
		for j := range block {
			t := float64(i*testBlock+j) / testRate
			block[j] = float32(a * math.Sin(2*math.Pi*f*t))
		}
		s.Capture(block)
		if i%3 == 2 {
			s.Update()
		}
	}
	s.Update()
}

func TestBandMapping(t *testing.T) {
	t.Parallel()
	if lo, _ := BandEdges(0); math.Abs(lo-LowHz) > 1e-9 {
		t.Errorf("first band starts at %v Hz, want %v", lo, LowHz)
	}
	if _, hi := BandEdges(Bands - 1); math.Abs(hi-HighHz) > 1e-6 {
		t.Errorf("last band ends at %v Hz, want %v", hi, HighHz)
	}
	s := New(testRate, testBlock)
	for b := range Bands {
		lo, hi := BandEdges(b)
		if b > 0 {
			if prevLo, prevHi := BandEdges(b - 1); math.Abs(prevHi-lo) > 1e-9 || math.Abs(hi/lo-prevHi/prevLo) > 1e-9 {
				t.Errorf("band %d (%v to %v Hz) is not the log step after band %d", b, lo, hi, b-1)
			}
		}
		if s.hi[b] <= s.lo[b] || s.hi[b] > Size/2+1 {
			t.Errorf("band %d has bins %d to %d", b, s.lo[b], s.hi[b])
		}
	}
	if p := Position(LowHz); p != 0 {
		t.Errorf("Position(LowHz) = %v", p)
	}
	if p := Position(HighHz); math.Abs(p-1) > 1e-12 {
		t.Errorf("Position(HighHz) = %v", p)
	}
}

func TestToneLandsInItsBand(t *testing.T) {
	t.Parallel()
	for _, band := range []int{0, 1, 3, 6, 10, 15, 20, 27} {
		lo, hi := BandEdges(band)
		f := math.Sqrt(lo * hi)
		s := New(testRate, testBlock)
		feed(s, f, 0.5, 0.5)
		best := 0
		for b, l := range s.Levels {
			if l > s.Levels[best] {
				best = b
			}
		}
		if best != band {
			t.Errorf("%.1f Hz: loudest band %d, want %d", f, best, band)
		}
		// A sine of amplitude 0.5 is -6 dBFS. Between bins, the Hann
		// window loses up to 1.4 dB.
		if l := s.Levels[best]; l < -7.6 || l > -5.5 {
			t.Errorf("%.1f Hz: band level %.2f dBFS, want -6", f, l)
		}
	}
}

func TestMeterSine(t *testing.T) {
	t.Parallel()
	s := New(testRate, testBlock)
	feed(s, 1000, 0.5, 0.5)
	if math.Abs(float64(s.RMS)+9.03) > 0.1 {
		t.Errorf("RMS %.2f dBFS, want -9.03", s.RMS)
	}
	if math.Abs(float64(s.Peak)+6.02) > 0.1 {
		t.Errorf("peak %.2f dBFS, want -6.02", s.Peak)
	}
	feed(s, 1000, 0.05, 0.5)
	if math.Abs(float64(s.Hold)+6.02) > 0.1 {
		t.Errorf("hold after 0.5 s %.2f dBFS, want -6.02", s.Hold)
	}
	if math.Abs(float64(s.Peak)+26.02) > 0.1 {
		t.Errorf("quiet peak %.2f dBFS, want -26.02", s.Peak)
	}
	feed(s, 1000, 0.05, 0.6)
	if math.Abs(float64(s.Hold)+26.02) > 0.1 {
		t.Errorf("hold after 1.1 s %.2f dBFS, want -26.02", s.Hold)
	}
}

func TestSilenceIsFloor(t *testing.T) {
	t.Parallel()
	s := New(testRate, testBlock)
	feed(s, 1000, 0, 0.2)
	if s.RMS != FloorDB || s.Peak != FloorDB || s.Levels[10] != FloorDB {
		t.Errorf("silence: RMS %v peak %v band %v, want %v", s.RMS, s.Peak, s.Levels[10], FloorDB)
	}
}

func TestNoAllocations(t *testing.T) {
	s := New(testRate, testBlock)
	block := make([]float32, testBlock)
	if a := testing.AllocsPerRun(100, func() {
		for range 7 {
			s.Capture(block)
		}
		s.Update()
	}); a != 0 {
		t.Errorf("%v allocations per Capture and Update, want 0", a)
	}
}

func TestReadWithScopeDoesNotAllocate(t *testing.T) {
	p, err := snarl.LoadProfile("v8")
	if err != nil {
		t.Fatal(err)
	}
	e, err := snarl.New(p, stream.SampleRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	e.Set(snarl.Snapshot{RPM: 3000, Throttle: 0.5, Load: 0.5, Flags: snarl.FlagRunning})
	src := stream.New()
	src.SetEngine(e)
	s := New(stream.SampleRate, stream.Block)
	src.SetTap(s)
	buf := make([]byte, stream.BlockBytes)
	_, _ = src.Read(buf)
	if a := testing.AllocsPerRun(100, func() {
		if n, _ := src.Read(buf); n != stream.BlockBytes {
			t.Fatalf("Read = %d", n)
		}
	}); a != 0 {
		t.Errorf("%v allocations per Read with the scope tap, want 0", a)
	}
	s.Update()
	if s.RMS <= FloorDB {
		t.Errorf("scope RMS %v after engine blocks, want above the floor", s.RMS)
	}
}

func BenchmarkUpdate(b *testing.B) {
	s := New(testRate, testBlock)
	block := make([]float32, testBlock)
	for i := range block {
		block[i] = float32(math.Sin(float64(i)))
	}
	b.ReportAllocs()
	for b.Loop() {
		// One game tick at 60 ticks per second is about 3 blocks; every
		// second tick computes a spectrum.
		for range 3 {
			s.Capture(block)
		}
		s.Update()
	}
}
