package main

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl"
	"github.com/oval-tutu/snarl/internal/scenario"
)

func TestRenderWritesWAV(t *testing.T) {
	dir := t.TempDir()
	scn := filepath.Join(dir, "short.scn")
	if err := os.WriteFile(scn, []byte("sweep 1000 3000 1 0.1\nhold 3000 0 0.05\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "a", "short.wav")
	args := []string{"--out", out, "--rate", "8000", "--block", "64", "--settle", "0.01", "flat12", scn}
	if err := run(args, io.Discard); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	const frames = 1200 // 0.15 s at 8000 Hz, not a whole number of blocks.
	if len(b) != 44+frames*2 {
		t.Fatalf("file size = %d, want %d", len(b), 44+frames*2)
	}
	if string(b[0:4]) != "RIFF" || string(b[36:40]) != "data" {
		t.Fatal("not a WAV file")
	}
	if got := binary.LittleEndian.Uint32(b[40:]); got != frames*2 {
		t.Errorf("data size = %d, want %d", got, frames*2)
	}
}

func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	scn := filepath.Join(dir, "s.scn")
	if err := os.WriteFile(scn, []byte("hold 1000 0.5 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o.wav")
	for _, args := range [][]string{
		{"flat12"},
		{"--out", out, "nope", scn},
		{"--out", out, "--tier", "3", "flat12", scn},
		{"--out", out, "--format", "s24", "flat12", scn},
		{"--out", out, "--block", "0", "flat12", scn},
		{"--out", out, "flat12", filepath.Join(dir, "missing.scn")},
	} {
		if err := run(args, io.Discard); err == nil {
			t.Errorf("run(%q) did not fail", args)
		}
	}
}

func TestSnapshot(t *testing.T) {
	scn, err := scenario.Parse(strings.NewReader("hold 2000 0.04 1\nhold 3000 0.5 1\n"))
	if err != nil {
		t.Fatal(err)
	}

	got := snapshot(scn, 0.5)
	want := snarl.Snapshot{RPM: 2000, Throttle: 0.04, Load: 0.04, Flags: snarl.FlagRunning | snarl.FlagOverrun}
	if got != want {
		t.Errorf("snapshot(0.5) = %+v, want %+v", got, want)
	}
	got = snapshot(scn, 1.5)
	want = snarl.Snapshot{RPM: 3000, Throttle: 0.5, Load: 0.5, Flags: snarl.FlagRunning}
	if got != want {
		t.Errorf("snapshot(1.5) = %+v, want %+v", got, want)
	}
}
