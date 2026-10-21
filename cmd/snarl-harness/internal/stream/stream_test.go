package stream

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/oval-tutu/snarl"
)

func newEngine(t *testing.T, name string) snarl.Engine {
	t.Helper()
	p, err := snarl.LoadProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	e, err := snarl.New(p, SampleRate, snarl.Tier1)
	if err != nil {
		t.Fatal(err)
	}
	e.Set(snarl.Snapshot{RPM: 3000, Throttle: 0.6, Load: 0.5, Flags: snarl.FlagRunning})
	return e
}

// wantFinite fails when a sample of the block is not finite or the two
// channels differ.
func wantFinite(t *testing.T, b []byte) (peak float64) {
	t.Helper()
	for i := 0; i < len(b); i += 8 {
		l := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
		r := math.Float32frombits(binary.LittleEndian.Uint32(b[i+4:]))
		if math.IsNaN(float64(l)) || math.IsInf(float64(l), 0) || l != r {
			t.Fatalf("frame %d: left %v, right %v", i/8, l, r)
		}
		peak = max(peak, math.Abs(float64(l)))
	}
	return peak
}

func TestReadReturnsOneBlock(t *testing.T) {
	s := New()
	buf := make([]byte, 4*BlockBytes)
	n, err := s.Read(buf)
	if err != nil || n != BlockBytes {
		t.Fatalf("silent Read = %d, %v, want %d", n, err, BlockBytes)
	}
	if wantFinite(t, buf[:n]) != 0 {
		t.Error("sound without an engine")
	}
	s.SetEngine(newEngine(t, "v12-merlin"))
	peak := 0.0
	for range 200 {
		n, err := s.Read(buf)
		if err != nil || n != BlockBytes {
			t.Fatalf("Read = %d, %v, want %d", n, err, BlockBytes)
		}
		peak = max(peak, wantFinite(t, buf[:n]))
	}
	if peak == 0 {
		t.Error("silence from a running engine")
	}
	s.SetEngine(newEngine(t, "v8-ls"))
	for range 8 {
		n, _ := s.Read(buf)
		wantFinite(t, buf[:n])
	}
	if s.old != nil || s.fade > 0 {
		t.Error("the crossfade did not end")
	}
	st := s.Stats()
	if st.Blocks != 209 || st.Max <= 0 || st.Mean <= 0 || st.Mean > st.Max {
		t.Errorf("stats %+v", st)
	}
	s.Reset()
	if _, err := s.Read(buf); err != nil || s.Stats().Blocks != 1 || s.Stats().MaxGap != 0 {
		t.Errorf("stats after Reset %+v", s.Stats())
	}
}

func TestReadShortSlices(t *testing.T) {
	s := New()
	s.SetEngine(newEngine(t, "i4-vtec"))
	small := make([]byte, 1000)
	total := 0
	for total < BlockBytes {
		n, err := s.Read(small)
		if err != nil || n == 0 {
			t.Fatalf("Read = %d, %v", n, err)
		}
		total += n
	}
	if total != BlockBytes || s.Stats().Blocks != 1 {
		t.Errorf("%d bytes over %d blocks, want one block", total, s.Stats().Blocks)
	}
}

func TestReadDoesNotAllocate(t *testing.T) {
	s := New()
	s.SetEngine(newEngine(t, "f6-aircooled"))
	buf := make([]byte, BlockBytes)
	_, _ = s.Read(buf)
	s.SetEngine(newEngine(t, "v12-merlin")) // The first runs include the crossfade.
	if a := testing.AllocsPerRun(100, func() {
		if n, _ := s.Read(buf); n != BlockBytes {
			t.Fatalf("Read = %d", n)
		}
	}); a != 0 {
		t.Errorf("%v allocations per Read, want 0", a)
	}
}

// copyTap keeps the last block it receives.
type copyTap struct {
	last  [Block]float32
	count int
}

func (c *copyTap) Capture(mono []float32) {
	copy(c.last[:], mono)
	c.count++
}

func TestTapGetsPlayedAudio(t *testing.T) {
	s := New()
	s.SetEngine(newEngine(t, "v8-ls"))
	tap := &copyTap{}
	s.SetTap(tap)
	buf := make([]byte, BlockBytes)
	for range 3 {
		if n, _ := s.Read(buf); n != BlockBytes {
			t.Fatalf("Read = %d", n)
		}
	}
	for i, v := range tap.last {
		if b := math.Float32bits(v); b != binary.LittleEndian.Uint32(buf[8*i:]) {
			t.Fatalf("frame %d: tap %v differs from the played sample", i, v)
		}
	}
	s.SetTap(nil)
	_, _ = s.Read(buf)
	if tap.count != 3 {
		t.Errorf("tap got %d blocks, want 3", tap.count)
	}
}

func TestReadWithTapDoesNotAllocate(t *testing.T) {
	s := New()
	s.SetEngine(newEngine(t, "f6-aircooled"))
	s.SetTap(&copyTap{})
	buf := make([]byte, BlockBytes)
	_, _ = s.Read(buf)
	if a := testing.AllocsPerRun(100, func() {
		if n, _ := s.Read(buf); n != BlockBytes {
			t.Fatalf("Read = %d", n)
		}
	}); a != 0 {
		t.Errorf("%v allocations per Read with a tap, want 0", a)
	}
}
