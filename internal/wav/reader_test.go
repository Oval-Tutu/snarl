package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestReadStereoS16WithExtraChunk(t *testing.T) {
	le := binary.LittleEndian
	samples := []int16{0, 16384, -32768, 32767, 100, -100}
	var b []byte
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, 0)
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint32(b, 44100)
	b = le.AppendUint32(b, 44100*4)
	b = le.AppendUint16(b, 4)
	b = le.AppendUint16(b, 16)
	b = append(b, "LIST"...)
	b = le.AppendUint32(b, 3)
	b = append(b, 'a', 'b', 'c', 0)
	b = append(b, "data"...)
	b = le.AppendUint32(b, 12)
	b, err := binary.Append(b, le, samples)
	if err != nil {
		t.Fatal(err)
	}

	a, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != 44100 || len(a.Channels) != 2 || a.Len() != 3 {
		t.Fatalf("got %d Hz, %d channels, %d frames", a.SampleRate, len(a.Channels), a.Len())
	}
	wantL := []float64{0, -1, 100.0 / 32768}
	wantR := []float64{0.5, 32767.0 / 32768, -100.0 / 32768}
	for i := range wantL {
		if a.Channels[0][i] != wantL[i] || a.Channels[1][i] != wantR[i] {
			t.Errorf("frame %d: got %v %v, want %v %v", i, a.Channels[0][i], a.Channels[1][i], wantL[i], wantR[i])
		}
	}
	if m := a.Mono(); m[0] != 0.25 {
		t.Errorf("mono frame 0: got %v, want 0.25", m[0])
	}
}

func TestReadF32RoundTrip(t *testing.T) {
	samples := []float32{0, 0.25, -0.75, 1.5}
	var buf bytes.Buffer
	w, err := NewWriter(&buf, 48000, F32, len(samples))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(samples); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	a, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != 48000 || len(a.Channels) != 1 || a.Len() != len(samples) {
		t.Fatalf("got %d Hz, %d channels, %d frames", a.SampleRate, len(a.Channels), a.Len())
	}
	for i, s := range samples {
		if math.Abs(a.Channels[0][i]-float64(s)) > 0 {
			t.Errorf("sample %d: got %v, want %v", i, a.Channels[0][i], s)
		}
	}
}

func TestReadRejectsUnsupported(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, 48000, S16, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	binary.LittleEndian.PutUint16(b[34:], 24)
	if _, err := Read(bytes.NewReader(b)); err == nil {
		t.Error("24-bit PCM: want an error")
	}
	if _, err := Read(bytes.NewReader([]byte("RIFX0000WAVE"))); err == nil {
		t.Error("RIFX: want an error")
	}
}
