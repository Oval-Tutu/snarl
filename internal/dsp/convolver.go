package dsp

import "fmt"

// Convolver is a uniformly partitioned overlap-save FFT convolution of a
// signal with a fixed impulse response in float32. It splits the response
// into partitions of one block, keeps the spectra of the last input blocks
// in a frequency-domain delay line, and per block does one forward FFT, one
// multiply-accumulate per partition and one inverse FFT, all of length two
// blocks. The output of a block is the output of the direct convolution for
// the same input samples, so the convolution adds no delay beyond the
// buffering of one block. Process does not allocate.
type Convolver struct {
	block, parts int
	fft          *RealFFT
	// h holds the spectra of the partitions, divided by the FFT length,
	// one after the other with block+1 bins each.
	h []complex64
	// fdl is the delay line of the input spectra, a ring of parts entries
	// of block+1 bins with the newest at head.
	fdl  []complex64
	head int
	in   []float32
	out  []float32
	acc  []complex64
}

// NewConvolver returns a convolution with the impulse response ir in
// blocks of block samples, a power of two of at least 4.
func NewConvolver(ir []float32, block int) (*Convolver, error) {
	if len(ir) == 0 {
		return nil, fmt.Errorf("dsp: empty impulse response")
	}
	fft, err := NewRealFFT(2 * block)
	if err != nil {
		return nil, fmt.Errorf("dsp: convolution block %d: %w", block, err)
	}
	parts := (len(ir) + block - 1) / block
	bins := block + 1
	c := &Convolver{
		block: block,
		parts: parts,
		fft:   fft,
		h:     make([]complex64, parts*bins),
		fdl:   make([]complex64, parts*bins),
		in:    make([]float32, 2*block),
		out:   make([]float32, 2*block),
		acc:   make([]complex64, bins),
	}
	scale := 1 / float32(2*block)
	pad := make([]float32, 2*block)
	for p := range parts {
		clear(pad)
		for i := range block {
			if j := p*block + i; j < len(ir) {
				pad[i] = ir[j] * scale
			}
		}
		fft.Forward(pad, c.h[p*bins:(p+1)*bins])
	}
	return c, nil
}

// Block returns the block length.
func (c *Convolver) Block() int { return c.block }

// Partitions returns the number of partitions of the impulse response.
func (c *Convolver) Partitions() int { return c.parts }

// Reset clears the input history.
func (c *Convolver) Reset() {
	clear(c.fdl)
	clear(c.in)
	c.head = 0
}

// Process convolves the next block x and writes the output block to y.
// Both have the block length. x and y may be the same slice.
func (c *Convolver) Process(x, y []float32) {
	b := c.block
	bins := b + 1
	copy(c.in[b:], x[:b])
	c.fft.Forward(c.in, c.fdl[c.head*bins:(c.head+1)*bins])
	copy(c.in[:b], c.in[b:])

	acc := c.acc
	clear(acc)
	idx := c.head
	for p := range c.parts {
		xs := c.fdl[idx*bins : (idx+1)*bins]
		hs := c.h[p*bins : (p+1)*bins]
		xs = xs[:len(acc)]
		hs = hs[:len(acc)]
		for k, a := range acc {
			acc[k] = a + cmul(xs[k], hs[k])
		}
		idx--
		if idx < 0 {
			idx = c.parts - 1
		}
	}
	c.head++
	if c.head == c.parts {
		c.head = 0
	}

	c.fft.Inverse(acc, c.out)
	copy(y[:b], c.out[b:])
}
