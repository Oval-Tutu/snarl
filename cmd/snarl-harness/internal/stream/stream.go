// Package stream turns a snarl engine into the io.Reader of an Ebitengine
// float32 player. Each Read renders at most one block, so the player buffer
// stays near its set size.
package stream

import (
	"encoding/binary"
	"math"
	"sync/atomic"
	"time"

	"github.com/oval-tutu/snarl"
)

// Output format.
const (
	SampleRate = 48000
	Block      = 256 // Frames per block.
	BlockBytes = Block * 2 * 4

	// BlockTime is the duration of one block.
	BlockTime = time.Duration(Block) * time.Second / SampleRate

	fadeFrames = 4 * Block // Length of the crossfade after an engine swap.
)

// holder lets an atomic.Pointer carry an Engine.
type holder struct{ e snarl.Engine }

// Tap receives a copy of each block that the source plays, as mono
// samples, on the audio goroutine. Capture must not allocate or wait.
type Tap interface {
	Capture(mono []float32)
}

// tapHolder lets an atomic.Pointer carry a Tap.
type tapHolder struct{ t Tap }

// Stats are the render timings. Max values count from the last Reset.
type Stats struct {
	Blocks    int64         // Blocks rendered.
	Last      time.Duration // Render time of the last block.
	Mean, Max time.Duration // Mean and largest render time per block.
	MaxGap    time.Duration // Largest interval between two Read calls.
}

// Source is the io.Reader of the player. SetEngine, Stats and Reset are
// safe on any goroutine. Read runs on the audio goroutine and does not
// allocate.
type Source struct {
	next atomic.Pointer[holder]
	tap  atomic.Pointer[tapHolder]

	// The audio goroutine owns these.
	cur, old *holder
	fade     int // Frames of the crossfade from old to cur that are left.
	mono     [Block]float32
	oldMono  [Block]float32
	out      [BlockBytes]byte
	pos      int // Bytes of out that Read returned.
	lastRead time.Time

	reset                        atomic.Bool
	blocks, lastNS, sumNS, maxNS atomic.Int64
	maxGapNS                     atomic.Int64
}

// New returns a Source that plays silence until SetEngine.
func New() *Source { return &Source{pos: BlockBytes} }

// SetEngine makes e the engine of the next block. The sound crossfades from
// the previous engine over four blocks. Build e and call e.Set before
// SetEngine, outside the audio goroutine.
func (s *Source) SetEngine(e snarl.Engine) { s.next.Store(&holder{e}) }

// SetTap makes t receive each block from the next block on. A nil t stops
// the copies. A Capture call can still run when SetTap returns.
func (s *Source) SetTap(t Tap) {
	if t == nil {
		s.tap.Store(nil)
		return
	}
	s.tap.Store(&tapHolder{t})
}

// Reset clears the timings on the next Read.
func (s *Source) Reset() { s.reset.Store(true) }

// Stats returns the timings.
func (s *Source) Stats() Stats {
	st := Stats{
		Blocks: s.blocks.Load(),
		Last:   time.Duration(s.lastNS.Load()),
		Max:    time.Duration(s.maxNS.Load()),
		MaxGap: time.Duration(s.maxGapNS.Load()),
	}
	if st.Blocks > 0 {
		st.Mean = time.Duration(s.sumNS.Load() / st.Blocks)
	}
	return st
}

// Read copies the rest of the current block into p, or renders a new block
// when none is left. It returns interleaved stereo float32 little endian
// samples and never an error.
func (s *Source) Read(p []byte) (int, error) {
	now := time.Now()
	if s.reset.Swap(false) {
		s.blocks.Store(0)
		s.lastNS.Store(0)
		s.sumNS.Store(0)
		s.maxNS.Store(0)
		s.maxGapNS.Store(0)
		s.lastRead = time.Time{}
	}
	if !s.lastRead.IsZero() {
		if gap := int64(now.Sub(s.lastRead)); gap > s.maxGapNS.Load() {
			s.maxGapNS.Store(gap)
		}
	}
	s.lastRead = now
	if s.pos == BlockBytes {
		s.render()
		s.pos = 0
		d := int64(time.Since(now))
		s.lastNS.Store(d)
		s.sumNS.Add(d)
		s.blocks.Add(1)
		if d > s.maxNS.Load() {
			s.maxNS.Store(d)
		}
	}
	n := copy(p, s.out[s.pos:])
	s.pos += n
	return n, nil
}

// render fills out with the next block.
func (s *Source) render() {
	if n := s.next.Load(); n != s.cur {
		if s.cur != nil && n != nil {
			s.old, s.fade = s.cur, fadeFrames
		}
		s.cur = n
	}
	if s.cur == nil {
		clear(s.mono[:])
	} else {
		s.cur.e.Render(s.mono[:])
	}
	if s.fade > 0 {
		s.old.e.Render(s.oldMono[:])
		done := fadeFrames - s.fade
		for i, v := range s.mono {
			t := float32(done+i) / fadeFrames
			s.mono[i] = s.oldMono[i] + (v-s.oldMono[i])*t
		}
		s.fade -= Block
		if s.fade <= 0 {
			s.old = nil
		}
	}
	if t := s.tap.Load(); t != nil {
		t.t.Capture(s.mono[:])
	}
	for i, v := range s.mono {
		b := math.Float32bits(v)
		binary.LittleEndian.PutUint32(s.out[8*i:], b)
		binary.LittleEndian.PutUint32(s.out[8*i+4:], b)
	}
}
