package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	tagPCM        = 1
	tagFloat      = 3
	tagExtensible = 0xFFFE
)

// Audio is decoded PCM audio with samples in the range -1 to 1.
type Audio struct {
	Channels   [][]float64
	SampleRate int
}

// Len returns the number of frames.
func (a *Audio) Len() int {
	if len(a.Channels) == 0 {
		return 0
	}
	return len(a.Channels[0])
}

// Duration returns the length in seconds.
func (a *Audio) Duration() float64 {
	return float64(a.Len()) / float64(a.SampleRate)
}

// Mono returns the mean of all channels.
func (a *Audio) Mono() []float64 {
	if len(a.Channels) == 1 {
		return append([]float64(nil), a.Channels[0]...)
	}
	out := make([]float64, a.Len())
	scale := 1 / float64(len(a.Channels))
	for _, ch := range a.Channels {
		for i, v := range ch {
			out[i] += v * scale
		}
	}
	return out
}

// ReadFile reads a 16-bit integer or 32-bit float WAV file.
func ReadFile(path string) (*Audio, error) {
	f, err := os.Open(path) //nolint:gosec // The path is a user-supplied input file.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	a, err := Read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

// Read decodes a RIFF WAVE stream with 16-bit integer or 32-bit float
// samples and any number of channels.
func Read(r io.Reader) (*Audio, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF WAVE file")
	}
	var format, channels, bits int
	var rate int
	haveFmt := false
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return nil, errors.New("no data chunk")
		}
		id := string(ch[0:4])
		size := int64(binary.LittleEndian.Uint32(ch[4:8]))
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("fmt chunk too short")
			}
			buf := make([]byte, size+size%2)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, fmt.Errorf("read fmt chunk: %w", err)
			}
			format = int(binary.LittleEndian.Uint16(buf[0:2]))
			channels = int(binary.LittleEndian.Uint16(buf[2:4]))
			rate = int(binary.LittleEndian.Uint32(buf[4:8]))
			bits = int(binary.LittleEndian.Uint16(buf[14:16]))
			if format == tagExtensible {
				if size < 26 {
					return nil, errors.New("extensible fmt chunk too short")
				}
				format = int(binary.LittleEndian.Uint16(buf[24:26]))
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, errors.New("data chunk before fmt chunk")
			}
			return decodeData(r, size, format, channels, bits, rate)
		default:
			if _, err := io.CopyN(io.Discard, r, size+size%2); err != nil {
				return nil, fmt.Errorf("skip %q chunk: %w", id, err)
			}
		}
	}
}

func decodeData(r io.Reader, size int64, format, channels, bits, rate int) (*Audio, error) {
	if channels < 1 || rate < 1 {
		return nil, fmt.Errorf("invalid format: %d channels at %d Hz", channels, rate)
	}
	var width int
	switch {
	case format == tagPCM && bits == 16:
		width = 2
	case format == tagFloat && bits == 32:
		width = 4
	default:
		return nil, fmt.Errorf("unsupported sample format %d with %d bits, want 16-bit PCM or 32-bit float", format, bits)
	}
	data, err := io.ReadAll(io.LimitReader(r, size))
	if err != nil {
		return nil, fmt.Errorf("read data chunk: %w", err)
	}
	frames := len(data) / (width * channels)
	a := &Audio{SampleRate: rate, Channels: make([][]float64, channels)}
	for c := range a.Channels {
		a.Channels[c] = make([]float64, frames)
	}
	for i := range frames {
		for c := range channels {
			off := (i*channels + c) * width
			var v float64
			if width == 2 {
				n := int(binary.LittleEndian.Uint16(data[off:]))
				if n >= 1<<15 {
					n -= 1 << 16
				}
				v = float64(n) / 32768
			} else {
				v = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off:])))
			}
			a.Channels[c][i] = v
		}
	}
	return a, nil
}
