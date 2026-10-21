package record

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oval-tutu/snarl"
)

const block = 256

// fill sets the block to the sequence n, n+1, ...
func fill(b []float32, n int) {
	for i := range b {
		b[i] = float32(n + i)
	}
}

func TestRingKeepsOrderAtNormalRate(t *testing.T) {
	r := NewRing(8, block)
	in, out := make([]float32, block), make([]float32, block)
	var wg sync.WaitGroup
	const blocks = 20000
	wg.Go(func() {
		for k := 0; k < blocks; {
			if r.Pop(out) {
				for i, v := range out {
					if v != float32(k*block+i) {
						t.Errorf("block %d sample %d = %v", k, i, v)
						break
					}
				}
				k++
			}
		}
	})
	for k := range blocks {
		// The producer is slower than the consumer, as the audio
		// goroutine is slower than the writer: it never fills the ring.
		for r.head.Load()-r.tail.Load() >= 4 {
			runtime.Gosched()
		}
		fill(in, k*block)
		if !r.Push(in) {
			t.Fatalf("Push %d failed", k)
		}
	}
	wg.Wait()
	if r.Dropped() != 0 {
		t.Errorf("%d blocks dropped", r.Dropped())
	}
}

func TestRingCountsDropsWhenConsumerStalls(t *testing.T) {
	r := NewRing(4, block)
	in := make([]float32, block)
	for k := range 10 {
		fill(in, k*block)
		if got := r.Push(in); got != (k < 4) {
			t.Errorf("Push %d = %v", k, got)
		}
	}
	if r.Dropped() != 6 {
		t.Errorf("%d blocks dropped, want 6", r.Dropped())
	}
	out := make([]float32, block)
	for k := range 4 {
		if !r.Pop(out) || out[0] != float32(k*block) {
			t.Fatalf("Pop %d = %v", k, out[0])
		}
	}
	if r.Pop(out) {
		t.Error("Pop from an empty ring")
	}
}

func TestRingPushDoesNotAllocate(t *testing.T) {
	r := NewRing(4, block)
	in, out := make([]float32, block), make([]float32, block)
	if a := testing.AllocsPerRun(100, func() {
		r.Push(in)
		r.Pop(out)
	}); a != 0 {
		t.Errorf("%v allocations, want 0", a)
	}
}

// after returns the part of a log row after the time.
func after(row string) string {
	_, rest, _ := strings.Cut(row, ",")
	return rest
}

func TestRecorderWritesLogAndAudio(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rec")
	r, err := Start(dir, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), 48000, block)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Start(dir, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), 48000, block); err == nil {
		t.Error("a second recording overwrote the first")
	}
	s := snarl.Snapshot{RPM: 6000.5, Throttle: 1, Load: 0.25, Speed: 30, Gear: 1, Flags: snarl.FlagRunning, Limiter: 3}
	r.Snapshot("v8-ls", snarl.Tier1, s)
	in := make([]float32, block)
	const blocks = 50
	for k := range blocks {
		fill(in, k*block)
		r.Capture(in)
	}
	res, err := r.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if res.Frames != blocks*block || res.Dropped != 0 {
		t.Errorf("result %v", res)
	}

	csv, err := os.ReadFile(r.CSVPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(csv)), "\n")
	if len(lines) != 3 || lines[0] != Header || !strings.HasSuffix(lines[1], ",v8-ls,1,6000.5,1,0.25,30,1,1,0,0,3,0,0,0") ||
		after(lines[2]) != after(lines[1]) {
		t.Errorf("log:\n%s", csv)
	}

	wav, err := os.ReadFile(r.WAVPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) != 58+4*blocks*block || !bytes.Equal(wav[:4], []byte("RIFF")) ||
		int(binary.LittleEndian.Uint32(wav[4:])) != len(wav)-8 ||
		binary.LittleEndian.Uint32(wav[46:]) != blocks*block ||
		binary.LittleEndian.Uint32(wav[54:]) != 4*blocks*block {
		t.Fatalf("WAV of %d bytes, header %x", len(wav), wav[:58])
	}
	for i := range blocks * block {
		if v := math.Float32frombits(binary.LittleEndian.Uint32(wav[58+4*i:])); v != float32(i) {
			t.Fatalf("sample %d = %v", i, v)
		}
	}
}
