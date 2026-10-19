package dsp

// Delay is a circular delay line with a length that is a power of two.
type Delay struct {
	buf  []float32
	mask int
	w    int
}

// NewDelay returns a delay line that can read at least max samples back.
func NewDelay(maxDelay int) *Delay {
	n := 2
	for n < maxDelay+2 {
		n <<= 1
	}
	return &Delay{buf: make([]float32, n), mask: n - 1}
}

// Write appends one sample.
func (d *Delay) Write(x float32) {
	d.w = (d.w + 1) & d.mask
	d.buf[d.w] = x
}

// Read returns the sample written n writes ago. Read(0) is the newest.
func (d *Delay) Read(n int) float32 {
	return d.buf[(d.w-n)&d.mask]
}

// Reset clears the delay line.
func (d *Delay) Reset() {
	clear(d.buf)
}
