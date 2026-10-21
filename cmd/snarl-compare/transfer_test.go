package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

// writeFloatWAV writes x as a mono float WAV file.
func writeFloatWAV(t *testing.T, path string, rate int, x []float64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := make([]float32, len(x))
	for i, v := range x {
		s[i] = float32(v)
	}
	ww, err := wav.NewWriter(f, rate, wav.F32, len(s))
	if err != nil {
		t.Fatal(err)
	}
	if err := ww.Write(s); err != nil {
		t.Fatal(err)
	}
	if err := ww.Close(); err != nil {
		t.Fatal(err)
	}
}

const testDump = `{
  "simulation_frequency_hz": 10000,
  "exhaust_systems": [
    { "length_m": 3.43, "audio_volume": 2, "impulse_response": "a.wav", "impulse_response_volume": 0.5 },
    { "length_m": 1.715, "audio_volume": 2, "impulse_response": "a.wav", "impulse_response_volume": 0.5 }
  ],
  "cylinders": [
    { "exhaust_system": 0, "header_primary_length_m": 0, "sound_attenuation": 1 },
    { "exhaust_system": 1, "header_primary_length_m": 0, "sound_attenuation": 0.5 }
  ]
}`

// TestReadInput checks the gains and the delays of the sound tap: cylinder
// 0 has 3.43 m of exhaust, a delay of 100 steps and a gain of
// 1600*2/2/3.43^2*0.5, and cylinder 1 has half the length, a delay of 50
// steps and a gain of 1600*0.5*2/2/1.715^2*0.5.
func TestReadInput(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "engine.json")
	if err := os.WriteFile(dump, []byte(testDump), 0o644); err != nil {
		t.Fatal(err)
	}
	taps, rate, err := readDump(dump)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 10000 || len(taps) != 2 || taps[0].delay != 100 || taps[1].delay != 50 {
		t.Fatalf("got rate %v and taps %+v", rate, taps)
	}
	g0, g1 := 1600/(3.43*3.43)*0.5, 1600*0.5/(1.715*1.715)*0.5
	if math.Abs(taps[0].gain-g0) > 1e-9 || math.Abs(taps[1].gain-g1) > 1e-9 {
		t.Fatalf("gains %v and %v, want %v and %v", taps[0].gain, taps[1].gain, g0, g1)
	}
	// Ten rows before time 0, then an impulse of 1 Pa in cylinder 0 at
	// step 0 and of 2 Pa in cylinder 1 at step 1.
	var b strings.Builder
	b.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
	for s := -10; s < 300; s++ {
		p0, p1 := atmospherePa, atmospherePa
		if s == 0 {
			p0++
		}
		if s == 1 {
			p1 += 2
		}
		fmt.Fprintf(&b, "%.4f,1000,%d,%d\n", float64(s)/10000, p0, p1)
	}
	x, err := readInput(strings.NewReader(b.String()), taps, rate)
	if err != nil {
		t.Fatal(err)
	}
	if len(x) != 300 {
		t.Fatalf("got %d samples, want 300", len(x))
	}
	for s, v := range x {
		want := 0.0
		switch s {
		case 100:
			want = g0
		case 51:
			want = 2 * g1
		}
		if math.Abs(v-want) > 1e-9 {
			t.Errorf("sample %d: %v, want %v", s, v, want)
		}
	}
	sparse := "time_s,runner_pressure_pa_0,runner_pressure_pa_1\n0,1,1\n0.01,1,1\n0.02,1,1\n"
	if _, err := readInput(strings.NewReader(sparse), taps, rate); err == nil || !strings.Contains(err.Error(), "--csv-every 1") {
		t.Errorf("CSV with every 100th step: got %v, want an error that names --csv-every 1", err)
	}
}

// TestTransfers renders a CSV of random runner pressures, builds a WAV
// output of the synthesiser input at 48 kHz scaled by -20 dB and 2 ms late,
// and checks the gain and the noise of each hold.
func TestTransfers(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "engine.json")
	scn := filepath.Join(dir, "holds.scn")
	csvPath := filepath.Join(dir, "holds.csv")
	wavPath := filepath.Join(dir, "holds.wav")
	if err := os.WriteFile(dump, []byte(testDump), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scn, []byte("hold 1000 1 2\nhold 2000 0 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(9, 10)) //nolint:gosec // Seeded test signal, not security.
	var b strings.Builder
	b.WriteString("time_s,rpm,runner_pressure_pa_0,runner_pressure_pa_1\n")
	for s := range 40000 {
		fmt.Fprintf(&b, "%.4f,1000,%.3f,%.3f\n", float64(s)/10000, atmospherePa+100*rng.NormFloat64(), atmospherePa+100*rng.NormFloat64())
	}
	if err := os.WriteFile(csvPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	taps, rate, err := readDump(dump)
	if err != nil {
		t.Fatal(err)
	}
	x, err := readInput(strings.NewReader(b.String()), taps, rate)
	if err != nil {
		t.Fatal(err)
	}
	x = analysis.Resample(x, 10000, 48000)
	var peak float64
	for _, v := range x {
		peak = max(peak, math.Abs(v))
	}
	y := make([]float64, len(x))
	for i := 96; i < len(y); i++ {
		y[i] = 0.1 * x[i-96] / peak
	}
	writeFloatWAV(t, wavPath, 48000, y)
	var out strings.Builder
	cfg := &config{scenario: scn, engine: dump, transfer: csvPath, settle: 0.25}
	if err := transfers(&out, []string{wavPath}, cfg); err != nil {
		t.Fatal(err)
	}
	t.Log(out.String())
	if !strings.Contains(out.String(), "lags the input by 2.00 ms") {
		t.Error("no lag of 2.00 ms")
	}
	var rows [][]string
	for line := range strings.SplitSeq(out.String(), "\n") {
		if f := strings.Split(line, "|"); len(f) == 9 && strings.Contains(f[1], "hold") {
			rows = append(rows, f)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("got %d hold rows, want 2", len(rows))
	}
	want := 20 * math.Log10(0.1/peak)
	for i, r := range rows {
		var gain, noise float64
		if _, err := fmt.Sscan(r[5], &gain); err != nil || math.Abs(gain-want) > 0.2 {
			t.Errorf("hold %d: gain %q dB, want %.1f", i+1, r[5], want)
		}
		if _, err := fmt.Sscan(r[6], &noise); err != nil || noise > -30 {
			t.Errorf("hold %d: noise %q dB, want below -30", i+1, r[6])
		}
	}
	if err := transfers(&out, []string{wavPath}, &config{scenario: scn, transfer: csvPath}); err == nil {
		t.Error("no --engine: got no error")
	}
}
