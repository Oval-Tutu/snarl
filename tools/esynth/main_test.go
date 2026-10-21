package main

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/esynth"
	"github.com/oval-tutu/snarl/internal/wav"
)

func TestRunWritesPortOutput(t *testing.T) {
	dump := filepath.Join("..", "..", "testdata", "esynth", "i4-vtec-noise")
	out := filepath.Join(t.TempDir(), "port.wav")
	var stdout bytes.Buffer
	if err := run([]string{"--dump", dump, "--precision", "float32", "--out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); strings.Count(got, "identical") != 2 {
		t.Errorf("want two identical outputs, got:\n%s", got)
	}

	d, err := esynth.LoadDump(dump)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := d.ReadOutput()
	if err != nil {
		t.Fatal(err)
	}
	a, err := wav.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != 44100 || a.Len() != d.KeptSamples {
		t.Fatalf("%d samples at %d Hz, want %d at 44100 Hz", a.Len(), a.SampleRate, d.KeptSamples)
	}
	for i, v := range a.Channels[0] {
		if want := float64(ref[d.SettleSamples+i]) / 32768; v != want {
			t.Fatalf("sample %d is %g, want %g", i, v, want)
		}
	}
}

func TestRunRuntime(t *testing.T) {
	dump := filepath.Join("..", "..", "testdata", "esynth", "i4-vtec-quiet")
	for _, rate := range []string{"44100", "48000"} {
		out := filepath.Join(t.TempDir(), "runtime.wav")
		var stdout bytes.Buffer
		if err := run([]string{"--dump", dump, "--runtime", "--rate", rate, "--taps", "1024", "--out", out}, &stdout); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); !strings.Contains(got, "runtime at "+rate+" Hz, block 256, at most 1024 taps") || !strings.Contains(got, "after the pre-roll: ") {
			t.Errorf("unexpected report:\n%s", got)
		}
		a, err := wav.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if strconv.Itoa(a.SampleRate) != rate || a.Len() < 8000 {
			t.Errorf("%d samples at %d Hz, want a 0.2 s hold at %s Hz", a.Len(), a.SampleRate, rate)
		}
	}
}

func TestRunRejectsBadPrecision(t *testing.T) {
	err := run([]string{"--dump", "x", "--precision", "half"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown precision") {
		t.Errorf("got %v, want an unknown precision error", err)
	}
}
