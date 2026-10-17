// Package wav reads and writes WAV files in 16-bit PCM or 32-bit float
// format. The writer writes mono files. The reader reads any number of
// channels.
package wav

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Format is the sample format of a WAV file.
type Format uint8

// Sample formats.
const (
	// S16 is 16-bit signed integer PCM.
	S16 Format = iota
	// F32 is 32-bit IEEE float.
	F32
)

// ParseFormat returns the Format for "s16" or "f32".
func ParseFormat(s string) (Format, error) {
	switch s {
	case "s16":
		return S16, nil
	case "f32":
		return F32, nil
	}
	return 0, fmt.Errorf("unknown sample format %q, want s16 or f32", s)
}

// BytesPerSample returns the size of one mono sample in bytes.
func (f Format) BytesPerSample() int {
	if f == F32 {
		return 4
	}
	return 2
}

// Writer writes a mono WAV file with a frame count that is known in
// advance, so that the destination does not need to seek.
type Writer struct {
	w       *bufio.Writer
	format  Format
	frames  int
	written int
	buf     []byte
}

// NewWriter writes the WAV header for frames mono samples at the given
// sample rate. Write the samples with Write, then call Close.
func NewWriter(w io.Writer, sampleRate int, format Format, frames int) (*Writer, error) {
	if sampleRate <= 0 || sampleRate > math.MaxUint32/4 {
		return nil, fmt.Errorf("sample rate %d is not valid", sampleRate)
	}
	if frames < 0 || frames > (math.MaxUint32-64)/4 {
		return nil, fmt.Errorf("frame count %d is not valid", frames)
	}
	rate, n := uint32(sampleRate), uint32(frames)

	var tag, bps uint16
	fmtSize, extra := uint32(16), uint32(0)
	switch format {
	case S16:
		tag, bps = 1, 2
	case F32:
		// A non-PCM format has a cbSize field and a fact chunk.
		tag, bps, fmtSize, extra = 3, 4, 18, 12
	default:
		return nil, fmt.Errorf("sample format %d is not known", format)
	}
	data := n * uint32(bps)
	riff := 4 + (8 + fmtSize) + extra + (8 + data)

	h := make([]byte, 0, 64)
	h = append(h, "RIFF"...)
	h = binary.LittleEndian.AppendUint32(h, riff)
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, fmtSize)
	h = binary.LittleEndian.AppendUint16(h, tag)
	h = binary.LittleEndian.AppendUint16(h, 1)
	h = binary.LittleEndian.AppendUint32(h, rate)
	h = binary.LittleEndian.AppendUint32(h, rate*uint32(bps))
	h = binary.LittleEndian.AppendUint16(h, bps)
	h = binary.LittleEndian.AppendUint16(h, bps*8)
	if format == F32 {
		h = binary.LittleEndian.AppendUint16(h, 0)
		h = append(h, "fact"...)
		h = binary.LittleEndian.AppendUint32(h, 4)
		h = binary.LittleEndian.AppendUint32(h, n)
	}
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, data)

	bw := bufio.NewWriter(w)
	if _, err := bw.Write(h); err != nil {
		return nil, err
	}
	return &Writer{w: bw, format: format, frames: frames}, nil
}

// Write appends mono samples in the range -1 to 1. Samples outside the
// range are clipped in S16 format, and NaN becomes 0.
func (w *Writer) Write(samples []float32) error {
	if w.written+len(samples) > w.frames {
		return fmt.Errorf("write of %d samples exceeds the frame count %d", w.written+len(samples), w.frames)
	}
	n := len(samples) * w.format.BytesPerSample()
	if cap(w.buf) < n {
		w.buf = make([]byte, n)
	}
	b := w.buf[:n]
	for i, s := range samples {
		if w.format == F32 {
			binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(s))
			continue
		}
		v := math.Round(float64(max(-1, min(1, s))) * math.MaxInt16)
		switch {
		case math.IsNaN(v):
			v = 0
		case v < 0:
			v += 1 << 16 // two's complement
		}
		binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
	}
	w.written += len(samples)
	_, err := w.w.Write(b)
	return err
}

// Close flushes the buffered data. It does not close the destination. It
// returns an error when fewer samples were written than the header states.
func (w *Writer) Close() error {
	if err := w.w.Flush(); err != nil {
		return err
	}
	if w.written != w.frames {
		return errors.New("fewer samples written than the header states")
	}
	return nil
}
