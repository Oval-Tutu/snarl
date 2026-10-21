// Package scope measures the played audio for the harness display: a
// level meter and a spectrum of log-spaced bands. Capture runs on the audio
// goroutine and only copies the block into a ring. Update and the results
// belong to the game goroutine. Neither allocates.
package scope

import (
	"math"

	"github.com/oval-tutu/snarl/cmd/snarl-harness/internal/record"
)

// Spectrum and meter settings.
const (
	Size    = 4096    // FFT length in samples.
	Bands   = 28      // Spectrum bands.
	LowHz   = 30.0    // Lower edge of the first band.
	HighHz  = 16000.0 // Upper edge of the last band.
	FloorDB = -90     // Lowest level that Update reports, in dBFS.

	ringSlots = 64 // Blocks in the ring, enough for several game ticks.
	rmsMS     = 50 // Length of the RMS window.
	spectraPS = 30 // Largest number of spectra per second.

	// Smoothing of the band levels per spectrum: a level rises fast and
	// falls slowly.
	rise, fall = 0.6, 0.2
)

// Scope is the stream.Tap of the meter and the spectrum.
type Scope struct {
	ring  *record.Ring
	rate  int
	block []float32

	hist  []float32 // The latest Size samples, oldest at pos.
	pos   int
	fresh int // Samples since the last spectrum.

	holdLin  float32
	holdLeft int // Samples until the peak hold falls.

	window []float64
	buf    []complex128
	lo, hi [Bands]int // Bin range of each band, hi exclusive.

	// RMS, Peak and Hold are the meter levels in dBFS: the RMS of the
	// last 50 ms, the peak of the samples of the last Update, and the
	// largest peak, held for 1 s.
	RMS, Peak, Hold float32
	// Levels are the smoothed band levels in dBFS, the peak of the bins of
	// each band, scaled so that a full-scale sine is 0 dB.
	Levels [Bands]float32
}

// New returns a Scope for blocks of block samples at rate.
func New(rate, block int) *Scope {
	s := &Scope{
		ring:   record.NewRing(ringSlots, block),
		rate:   rate,
		block:  make([]float32, block),
		hist:   make([]float32, Size),
		window: make([]float64, Size),
		buf:    make([]complex128, Size),
		RMS:    FloorDB, Peak: FloorDB, Hold: FloorDB,
	}
	for i := range s.window {
		s.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/Size)
	}
	// A band has the bins whose centres are in it. A band narrower than a
	// bin with no centre in it has the bin nearest its centre.
	bin := float64(Size) / float64(rate)
	for b := range Bands {
		lo, hi := BandEdges(b)
		s.lo[b], s.hi[b] = int(math.Ceil(lo*bin)), int(math.Ceil(hi*bin))
		if s.hi[b] <= s.lo[b] {
			s.lo[b] = int(math.Round(math.Sqrt(lo*hi) * bin))
			s.hi[b] = s.lo[b] + 1
		}
	}
	for b := range s.Levels {
		s.Levels[b] = FloorDB
	}
	return s
}

// BandEdges returns the lower and upper edge of band b in Hz.
func BandEdges(b int) (lo, hi float64) {
	r := math.Log(HighHz/LowHz) / Bands
	return LowHz * math.Exp(r*float64(b)), LowHz * math.Exp(r*float64(b+1))
}

// Position returns where f falls on the log axis of the bands, from 0 at
// LowHz to 1 at HighHz.
func Position(f float64) float64 {
	return math.Log(f/LowHz) / math.Log(HighHz/LowHz)
}

// Capture copies one block into the ring. When the ring is full, it drops
// the block.
func (s *Scope) Capture(mono []float32) { s.ring.Push(mono) }

// Update reads the blocks that Capture queued, updates the meter, and
// updates the spectrum when 1/30 s of new samples is ready.
func (s *Scope) Update() {
	n := 0
	var peak float32
	for s.ring.Pop(s.block) {
		for _, v := range s.block {
			peak = max(peak, abs(v))
			s.hist[s.pos] = v
			s.pos = (s.pos + 1) % Size
		}
		n += len(s.block)
	}
	if n == 0 {
		return
	}
	s.Peak = dB(float64(peak) * float64(peak))
	s.holdLeft -= n
	if peak >= s.holdLin || s.holdLeft <= 0 {
		s.holdLin, s.holdLeft = peak, s.rate
	}
	s.Hold = dB(float64(s.holdLin) * float64(s.holdLin))
	m := min(s.rate*rmsMS/1000, Size)
	var sum float64
	for i := range m {
		v := float64(s.hist[(s.pos-1-i+Size)%Size])
		sum += v * v
	}
	s.RMS = dB(sum / float64(m))
	s.fresh += n
	if s.fresh >= s.rate/spectraPS {
		s.fresh = 0
		s.spectrum()
	}
}

// spectrum updates Levels from an FFT of the latest Size samples.
func (s *Scope) spectrum() {
	for i := range s.buf {
		s.buf[i] = complex(float64(s.hist[(s.pos+i)%Size])*s.window[i], 0)
	}
	FFT(s.buf)
	// A sine of amplitude a peaks at a*Size/4 under the Hann window.
	const scale = 16.0 / (Size * Size)
	for b := range Bands {
		var p float64
		for _, x := range s.buf[s.lo[b]:s.hi[b]] {
			p = max(p, real(x)*real(x)+imag(x)*imag(x))
		}
		d, l := dB(p*scale), s.Levels[b]
		k := float32(fall)
		if d > l {
			k = rise
		}
		s.Levels[b] = l + k*(d-l)
	}
}

// dB converts a power to decibels, no lower than FloorDB.
func dB(p float64) float32 {
	return float32(max(10*math.Log10(p), FloorDB))
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
