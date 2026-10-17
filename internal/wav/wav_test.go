package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestHeaderAndRoundTrip(t *testing.T) {
	tests := []struct {
		format             Format
		tag, bits, dataOff int
	}{
		{S16, 1, 16, 36},
		{F32, 3, 32, 50},
	}
	samples := []float32{0, 0.5, -0.5, 1, -1, 2, -2}
	for _, tc := range tests {
		var buf bytes.Buffer
		w, err := NewWriter(&buf, 44100, tc.format, len(samples))
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Write(samples[:3]); err != nil {
			t.Fatal(err)
		}
		if err := w.Write(samples[3:]); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		b := buf.Bytes()
		le := binary.LittleEndian
		bps := tc.bits / 8
		check := func(name string, got, want int) {
			t.Helper()
			if got != want {
				t.Errorf("format %d: %s = %d, want %d", tc.format, name, got, want)
			}
		}
		check("file size", len(b), tc.dataOff+8+len(samples)*bps)
		check("RIFF size", int(le.Uint32(b[4:])), len(b)-8)
		check("format tag", int(le.Uint16(b[20:])), tc.tag)
		check("channels", int(le.Uint16(b[22:])), 1)
		check("sample rate", int(le.Uint32(b[24:])), 44100)
		check("byte rate", int(le.Uint32(b[28:])), 44100*bps)
		check("block align", int(le.Uint16(b[32:])), bps)
		check("bits", int(le.Uint16(b[34:])), tc.bits)
		if string(b[0:4]) != "RIFF" || string(b[8:16]) != "WAVEfmt " || string(b[tc.dataOff:tc.dataOff+4]) != "data" {
			t.Fatalf("format %d: chunk IDs are wrong", tc.format)
		}
		data := int(le.Uint32(b[tc.dataOff+4:]))
		check("sample count", data/bps, len(samples))

		pcm := b[tc.dataOff+8:]
		if tc.format == S16 {
			// Two's complement of 0, 16384, -16384, 32767, -32767, 32767, -32767.
			want := []int{0, 16384, 49152, 32767, 32769, 32767, 32769}
			for i, v := range want {
				check("sample", int(le.Uint16(pcm[i*2:])), v)
			}
		} else {
			for i, v := range samples {
				if got := math.Float32frombits(le.Uint32(pcm[i*4:])); got != v {
					t.Errorf("sample %d = %v, want %v", i, got, v)
				}
			}
		}
	}
}

func TestFrameCountMismatch(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, 48000, S16, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(make([]float32, 3)); err == nil {
		t.Error("write past the frame count did not fail")
	}
	if err := w.Close(); err == nil {
		t.Error("close with missing samples did not fail")
	}
}
