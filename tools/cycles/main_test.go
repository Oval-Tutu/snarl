package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/cycles"
	"github.com/oval-tutu/snarl/internal/esynth"
	"github.com/oval-tutu/snarl/internal/scenario"
	"github.com/oval-tutu/snarl/internal/wav"
)

func TestGridRPMs(t *testing.T) {
	got := gridRPMs(690, 2000, 250)
	want := []float64{690, 1000, 1250, 1500, 1750, 2000}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("gridRPMs = %v, want %v", got, want)
	}
	// 760 is closer than a quarter step above idle, so the grid skips it
	// as 750 and starts at 1000.
	if got := gridRPMs(770, 1010, 250); fmt.Sprint(got) != "[770 1010]" {
		t.Errorf("gridRPMs = %v", got)
	}
}

func TestWithFine(t *testing.T) {
	got := withFine(gridRPMs(770, 3000, 250), 2000, 125)
	want := "[770 875 1000 1125 1250 1375 1500 1625 1750 1875 2000 2250 2500 2750 3000]"
	if fmt.Sprint(got) != want {
		t.Errorf("withFine = %v, want %s", got, want)
	}
	// 625 is closer than a quarter step to idle at 600.
	if got := withFine(gridRPMs(600, 1000, 250), 1000, 125); fmt.Sprint(got) != "[600 750 875 1000]" {
		t.Errorf("withFine = %v", got)
	}
	if got := withFine(gridRPMs(770, 3000, 250), 0, 125); len(got) != 10 {
		t.Errorf("withFine without fine rows = %v", got)
	}
}

func TestGridTop(t *testing.T) {
	for _, c := range []struct{ limiter, want float64 }{{9400, 9250}, {6500, 6250}, {18500, 18250}, {0, 0}} {
		if got := gridTop(c.limiter, 250); got != c.want {
			t.Errorf("gridTop(%g) = %g, want %g", c.limiter, got, c.want)
		}
	}
	// 0.998 of the limiter falls on a multiple of the step, so the top is
	// one step below it.
	if got := gridTop(9000/limiterShare, 250); got != 8750 {
		t.Errorf("gridTop on a step = %g, want 8750", got)
	}
}

func TestHoldSeconds(t *testing.T) {
	o := gridOptions{settle: 0.5, minHold: 1, cycles: 24}
	for _, c := range []struct{ rpm, want float64 }{{690, 4.7}, {6000, 1.5}, {1440, 2.5}} {
		if got := holdSeconds(c.rpm, o); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("holdSeconds(%g) = %g, want %g", c.rpm, got, c.want)
		}
	}
}

func TestScenariosWritesParsableRows(t *testing.T) {
	dir := t.TempDir()
	holds := filepath.Join(dir, "holds.scn")
	if err := os.WriteFile(holds, []byte("hold 700 1 3\nsweep 700 3000 1 0.5\nhold 3000 1 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "grid")
	var stdout bytes.Buffer
	if err := run([]string{"scenarios", "--holds", holds, "--name", "test", "--out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"t000", "t025", "t050", "t075", "t100"} {
		sc, err := scenario.ParseFile(filepath.Join(out, name+".scn"))
		if err != nil {
			t.Fatal(err)
		}
		var holdRPMs []float64
		for _, s := range sc.Segments {
			if s.Kind == scenario.Hold {
				holdRPMs = append(holdRPMs, s.RPM0)
			}
		}
		if fmt.Sprint(holdRPMs) != "[700 1000 1250 1500 1750 2000 2250 2500 2750 3000]" {
			t.Errorf("%s holds %v", name, holdRPMs)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "cut.scn"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\ncut "); n != 8 {
		t.Errorf("cut.scn has %d cut holds, want 8:\n%s", n, b)
	}
	sc, err := scenario.ParseFile(filepath.Join(out, "close.scn"))
	if err != nil {
		t.Fatal(err)
	}
	var closes []float64
	for i := 1; i < len(sc.Segments); i++ {
		a, b := sc.Segments[i-1], sc.Segments[i]
		if a.Kind == scenario.Hold && b.Kind == scenario.Hold && a.RPM0 == b.RPM0 && a.Throttle == 1 && b.Throttle == 0 {
			closes = append(closes, b.RPM0)
		}
	}
	if fmt.Sprint(closes) != "[700 1750 2750 3000]" {
		t.Errorf("close.scn closes at %v", closes)
	}

	// With the limiter, the rows go on above the last hold, and the
	// throttle step stays at 60 per cent of the last hold.
	stdout.Reset()
	if err := run([]string{"scenarios", "--holds", holds, "--name", "test", "--out", out, "--limiter", "3600"}, &stdout); err != nil {
		t.Fatal(err)
	}
	sc, err = scenario.ParseFile(filepath.Join(out, "t100.scn"))
	if err != nil {
		t.Fatal(err)
	}
	if last := sc.Segments[len(sc.Segments)-1]; last.RPM0 != 3500 {
		t.Errorf("t100.scn ends at %g rpm, want 3500", last.RPM0)
	}
	sc, err = scenario.ParseFile(filepath.Join(out, "step.scn"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Segments[0].RPM0 != 1750 {
		t.Errorf("step.scn at %g rpm, want 1750", sc.Segments[0].RPM0)
	}
}

func TestThinPoints(t *testing.T) {
	var ps []cycles.Point
	for _, th := range []float64{0, 1} {
		for _, r := range []float64{1000, 2000, 3000, 4000} {
			ps = append(ps, cycles.Point{RPM: r, Throttle: th, Ignition: true})
		}
	}
	got := thinPoints(sortPoints(ps), 2)
	var s []string
	for _, p := range got {
		s = append(s, fmt.Sprintf("%g/%g", p.Throttle, p.RPM))
	}
	if strings.Join(s, " ") != "0/1000 0/3000 0/4000 1/1000 1/3000 1/4000" {
		t.Errorf("thinPoints kept %v", s)
	}
}

// writeRecord writes a --dump-synth record of one 2 s hold at 3000 rpm and
// throttle 1 into dir: a periodic input at 10 kHz, the crank angle and
// speed, and an impulse response.
func writeRecord(t *testing.T, dir string) {
	t.Helper()
	const rate, rpm, settle, hold = 10000.0, 3000.0, 0.2, 2.0
	steps := int((settle + hold) * rate)
	le := binary.LittleEndian
	var in, crank []byte
	for i := range steps {
		phase := float64(i+1) / rate * rpm / 120
		v := math.Sin(2*math.Pi*4*phase) + 0.3*math.Cos(2*math.Pi*9*phase)
		in = le.AppendUint64(in, math.Float64bits(1000*v))
		angle := math.Mod(phase, 1) * 4 * math.Pi
		crank = le.AppendUint64(crank, math.Float64bits(angle))
		crank = le.AppendUint64(crank, math.Float64bits(-rpm*2*math.Pi/60))
	}
	frames := make([]map[string]any, steps/50)
	for i := range frames {
		frames[i] = map[string]any{"steps": 50, "blocks": [][2]int{{221, 0}}}
	}
	head := map[string]any{
		"engine": "Test", "script": "test.mr", "simulation_frequency_hz": rate, "input_sample_rate_hz": rate,
		"audio_sample_rate_hz": 44100, "input_channel_count": 1, "input_buffer_size": 44100, "audio_buffer_size": 44100,
		"audio_parameters": esynth.DefaultAudioParameters(),
		"channels": []map[string]any{{
			"input": "input_0.f64", "impulse_response": "ir.wav", "impulse_response_copy": "ir_0.wav",
			"impulse_response_file_samples": 64, "impulse_response_taps": 1, "impulse_response_volume": 0.01,
		}},
		"cylinders":  []map[string]any{{"exhaust_system": 0, "delay_steps": 30, "weight": 1}},
		"timestep_s": 1 / rate, "settle_s": settle,
		"segments": []map[string]any{{"mode": "hold", "rpm0": rpm, "rpm1": rpm, "throttle": 1, "seconds": hold}},
		"crank":    "crank.f64", "output": "output.wav", "output_samples": 0, "settle_samples": 0, "kept_samples": 0,
		"frames": frames,
	}
	j, err := json.Marshal(head)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"synth.json": j, "input_0.f64": in, "crank.f64": crank} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeIR(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w, err := wav.NewWriter(f, 44100, wav.S16, 64)
	if err != nil {
		t.Fatal(err)
	}
	ir := make([]float32, 64)
	ir[0] = 0.5
	if err := w.Write(ir); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAndPlay(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRecord(t, rec)
	tables := filepath.Join(dir, "test.bin")
	var stdout bytes.Buffer
	if err := run([]string{"build", "--out", tables, rec}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "1 points, 1 channels") {
		t.Fatalf("build report:\n%s", stdout.String())
	}
	tab, err := cycles.ReadFile(tables)
	if err != nil {
		t.Fatal(err)
	}
	p := tab.Points[0]
	if p.Harmonics != 200 || math.Abs(p.MeasuredRPM-3000) > 1 || tab.Channels[0].DelaySteps != 30 {
		t.Fatalf("point %g rpm, %d harmonics, delay %g", p.MeasuredRPM, p.Harmonics, tab.Channels[0].DelaySteps)
	}
	// Harmonic 4 has amplitude 1000: |c_4| = 500.
	if a := math.Hypot(p.Wave[0][7], p.Wave[0][8]); math.Abs(a-500) > 1 {
		t.Errorf("harmonic 4 magnitude %g, want 500", a)
	}

	es := filepath.Join(dir, "es")
	if err := os.MkdirAll(es, 0o755); err != nil {
		t.Fatal(err)
	}
	writeIR(t, filepath.Join(es, "ir.wav"))
	scn := filepath.Join(dir, "hold.scn")
	if err := os.WriteFile(scn, []byte("hold 3000 1 0.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "play")
	stdout.Reset()
	if err := run([]string{"play", "--tables", tables, "--es", es, "--out-dir", out, "--settle", "0.3", scn}, &stdout); err != nil {
		t.Fatal(err)
	}
	a, err := wav.ReadFile(filepath.Join(out, "hold.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if a.SampleRate != 44100 || a.Len() != 22050 {
		t.Errorf("%d samples at %d Hz, want 22050 at 44100", a.Len(), a.SampleRate)
	}
	var peak float64
	for _, v := range a.Channels[0] {
		peak = max(peak, math.Abs(v))
	}
	if peak == 0 {
		t.Error("the playback is silent")
	}

	stdout.Reset()
	if err := run([]string{"spectrum", "--record", rec, "--rpm", "3000", "--frame", "256", "--groups", "4"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stdout.String(), "\n| "); n != 6+2+cycles.BandCount+2+7 {
		t.Errorf("spectrum printed %d table rows, want a header, a rule and 4 groups, then a header, a rule and the bands, then a header, a rule and 7 part counts:\n%s", n, stdout.String())
	}
	// The modulation table has a row for 8 parts per cycle, the parts of
	// the tables.
	if !strings.Contains(stdout.String(), "| 8 | ") {
		t.Errorf("spectrum printed no modulation for 8 parts:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"wavspectrum", "--scenario", scn, "--rpm", "3000", "--settle", "0.1", "--frame", "1024", "--groups", "5", filepath.Join(out, "hold.wav")}, &stdout); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stdout.String(), "\n| "); n != 7 {
		t.Errorf("wavspectrum printed %d table rows, want a header, a rule and 5 groups:\n%s", n, stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"blend", "--tables", tables}, &stdout); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(stdout.String(), "\n| "); n != 2 {
		t.Errorf("blend printed %d table rows for one point, want a header and a rule:\n%s", n, stdout.String())
	}
}

func TestScoreDiff(t *testing.T) {
	dir := t.TempDir()
	write := func(name, rows string) string {
		path := filepath.Join(dir, name)
		text := "### test-engine\n\n| RPM | t000 | t100 |\n| --- | --- | --- |\n" + rows + "\n| RPM | mid-t100 |\n| --- | --- |\n| 1125 | 99.0 +0.2 +0.0 +0.0  |\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	before := write("before.md", "| 1000 | 100.0 +0.4 +0.0 +0.0  | 90.0 -0.4 +0.0 +0.0 S |\n| 1250 | 100.0 +1.0 +0.0 +0.0  | 99.0 -0.2 +0.0 +0.0  |\n")
	after := write("after.md", "| 1000 | 99.5 +0.4 +0.0 +0.0  | 100.0 -0.4 +0.0 +0.0  |\n| 1250 | 96.0 +1.0 +0.0 +0.0  | 99.0 +1.7 +0.0 +0.0 H |\n")
	var stdout bytes.Buffer
	if err := run([]string{"scorediff", before, after}, &stdout); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{"| test-engine t000 1250 rpm | 100.0 +1.0 | 96.0 +1.0 |", "| test-engine t100 1250 rpm | 99.0 -0.2 | 99.0 +1.7 |", "5 holds in both: 4 pass before, 4 after"} {
		if !strings.Contains(out, want) {
			t.Errorf("scorediff output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "t000 1000 rpm") {
		t.Errorf("scorediff lists a loss within the margin:\n%s", out)
	}
}

func TestRunErrors(t *testing.T) {
	var stdout bytes.Buffer
	for _, args := range [][]string{nil, {"nope"}, {"build"}, {"play"}, {"scenarios"}, {"spectrum"}} {
		if err := run(args, &stdout); err == nil {
			t.Errorf("run(%q) gave no error", args)
		}
	}
}
