package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
		{"--out", out, "--tier", "0", "flat12", scn},
		{"--out", out, "--tier", "3", "flat12", scn},
		{"--out", out, "--format", "s24", "flat12", scn},
		{"--out", out, "--block", "0", "flat12", scn},
		{"--out", out, "flat12", filepath.Join(dir, "missing.scn")},
		{"--out", out, "--set", "noise.bogus=1", "flat12", scn},
		{"--out", out, "--replay", filepath.Join(dir, "missing.csv")},
		{"--out", out, "--replay", scn, "flat12", scn},
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

// logText returns a snapshot log with one row per block of the scenario,
// at the time at the start of the block, and a last row at the end.
func logText(scn *scenario.Scenario, profile string, rate, block int) string {
	var b strings.Builder
	b.WriteString(logHeader + "\n")
	row := func(t float64) {
		s := snapshot(scn, t)
		f := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
		fmt.Fprintf(&b, "%s,%s,1,%s,%s,%s,%s,%d,%d,%d,%d,%d,%d,%s,%d\n",
			strconv.FormatFloat(t, 'g', -1, 64), profile, f(s.RPM), f(s.Throttle), f(s.Load), f(s.Speed),
			s.Gear, s.Flags, s.ShiftUp, s.ShiftDown, s.Limiter, s.PopRequest, f(s.PopStrength), s.PopCharacter)
	}
	frames := int(math.Round(scn.Duration() * float64(rate)))
	for pos := 0; pos < frames; pos += block {
		row(float64(pos) / float64(rate))
	}
	row(scn.Duration())
	return b.String()
}

func TestReplayMatchesScenario(t *testing.T) {
	dir := t.TempDir()
	const text = "hold 6000 1 0.2\nsweep 6000 1500 0 0.3\nhold 1500 0.3 0.1\n"
	scn, err := scenario.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	scnPath := filepath.Join(dir, "s.scn")
	logPath := filepath.Join(dir, "s.csv")
	if err := os.WriteFile(scnPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(logText(scn, "v8", 8000, 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func(args ...string) []byte {
		t.Helper()
		out := filepath.Join(dir, "out.wav")
		args = append([]string{"--out", out, "--rate", "8000", "--block", "64", "--format", "f32"}, args...)
		if err := run(args, io.Discard); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	want := read("v8", scnPath)
	first := read("--replay", logPath)
	if !bytes.Equal(first, want) {
		t.Error("the replay of the scenario log differs from the scenario render")
	}
	if !bytes.Equal(read("--replay", logPath), first) {
		t.Error("two replays of one log differ")
	}
	muted := read("--set", "noise.jet=0", "--set", "pops.density=0", "--replay", logPath)
	if len(muted) != len(want) || bytes.Equal(muted, want) {
		t.Error("--set did not change the replay")
	}
}

func TestReplayProfileChange(t *testing.T) {
	r := &replayer{rate: 8000, rows: []logRow{
		{t: 0, profile: "v8", tier: 1, snap: snarl.Snapshot{RPM: 3000, Flags: snarl.FlagRunning}},
		{t: 0.1, profile: "v8", tier: 1, snap: snarl.Snapshot{RPM: 3100, Flags: snarl.FlagRunning}},
		{t: 0.2, profile: "flat6", tier: 2, snap: snarl.Snapshot{RPM: 3200, Flags: snarl.FlagRunning}},
	}}
	e0, _, _ := r.at(0)
	e1, s1, _ := r.at(0.15)
	e2, s2, err := r.at(0.3)
	if err != nil || e0 != e1 || e1 == e2 || s1.RPM != 3100 || s2.RPM != 3200 {
		t.Errorf("engine kept %v, rebuilt %v, RPM %v and %v, error %v", e0 == e1, e1 != e2, s1.RPM, s2.RPM, err)
	}
	r.rows[2].profile = "nope"
	r.eng = nil
	if _, _, err := r.at(0.3); err == nil {
		t.Error("an unknown profile did not fail")
	}
}

func TestParseLogErrors(t *testing.T) {
	const row = "0,v8,1,3000,1,1,10,1,1,0,0,0,2,0.5,3\n"
	for name, text := range map[string]string{
		"short pop row": logHeader + "\n0,v8,1,3000,1,1,10,1,1,0,0,0\n",
		"bad character": logHeader + "\n0,v8,1,3000,1,1,10,1,1,0,0,0,2,0.5,256\n",
		"empty":         "",
		"bad header":    "t,profile\n" + row,
		"no rows":       logHeader + "\n",
		"falling":       logHeader + "\n0.5,v8,1,3000,1,1,10,1,1,0,0,0,0,0,0\n" + row,
		"tier 0":        logHeader + "\n0,v8,0,3000,1,1,10,1,1,0,0,0,0,0,0\n",
		"bad tier":      logHeader + "\n0,v8,3,3000,1,1,10,1,1,0,0,0,0,0,0\n",
		"bad number":    logHeader + "\n0,v8,1,fast,1,1,10,1,1,0,0,0,0,0,0\n",
		"bad byte":      logHeader + "\n0,v8,1,3000,1,1,10,1.5,1,0,0,0,0,0,0\n",
		"short row":     logHeader + "\n0,v8,1\n",
		"negative t":    logHeader + "\n-1,v8,1,3000,1,1,10,1,1,0,0,0,0,0,0\n",
	} {
		if _, err := parseLog(strings.NewReader(text)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	rows, err := parseLog(strings.NewReader(logHeader + "\n" + row))
	want := logRow{t: 0, profile: "v8", tier: 1, snap: snarl.Snapshot{
		RPM: 3000, Throttle: 1, Load: 1, Speed: 10, Gear: 1, Flags: snarl.FlagRunning,
		PopRequest: 2, PopStrength: 0.5, PopCharacter: snarl.CharacterRace,
	}}
	if err != nil || len(rows) != 1 || rows[0] != want {
		t.Errorf("parseLog = %+v, %v", rows, err)
	}
	// A log from before the pop columns has no pop requests.
	rows, err = parseLog(strings.NewReader(legacyHeader + "\n0,v8,1,3000,1,1,10,1,1,0,0,0\n"))
	want.snap.PopRequest, want.snap.PopStrength, want.snap.PopCharacter = 0, 0, 0
	if err != nil || len(rows) != 1 || rows[0] != want {
		t.Errorf("parseLog of a legacy log = %+v, %v", rows, err)
	}
}

// TestCatalogueProfile checks that PROFILE, --set and --replay take a
// catalogue id, with the catalogue that the default search finds, and
// that a catalogue id fails without the catalogue.
func TestCatalogueProfile(t *testing.T) {
	dir := t.TempDir()
	const text = "hold 2000 1 0.1\nsweep 2000 1000 0 0.1\n"
	scn, err := scenario.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	scnPath := filepath.Join(dir, "s.scn")
	logPath := filepath.Join(dir, "s.csv")
	if err := os.WriteFile(scnPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	const id = "el-v12-60-aero"
	if err := os.WriteFile(logPath, []byte(logText(scn, id, 8000, 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	read := func(args ...string) []byte {
		t.Helper()
		out := filepath.Join(dir, "out.wav")
		args = append([]string{"--out", out, "--rate", "8000", "--block", "64", "--format", "f32"}, args...)
		if err := run(args, io.Discard); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	want := read(id, scnPath)
	if !bytes.Equal(read("--replay", logPath), want) {
		t.Error("the replay of a catalogue log differs from the scenario render")
	}
	if !bytes.Equal(read("--catalogue", filepath.Join("..", "..", "profiles", "catalogue"), id, scnPath), want) {
		t.Error("--catalogue differs from the default search")
	}
	if bytes.Equal(read("--set", "noise.jet=0", "--set", "pops.density=0", id, scnPath), want) {
		t.Error("--set did not change a catalogue render")
	}
	out := filepath.Join(dir, "o.wav")
	for _, args := range [][]string{
		{"--out", out, "--catalogue", filepath.Join(dir, "missing"), id, scnPath},
		{"--out", out, "--catalogue", filepath.Join(dir, "missing"), "--replay", logPath},
		{"--out", out, "es-nope", scnPath},
		{"--out", out, "--set", "noise.bogus=1", id, scnPath},
	} {
		if err := run(args, io.Discard); err == nil {
			t.Errorf("run(%q) did not fail", args)
		}
	}
}
