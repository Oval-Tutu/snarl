package main

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

func writeTestWAV(t *testing.T, path string, r int, x []float64) {
	t.Helper()
	y := make([]float32, len(x))
	for i, v := range x {
		y[i] = float32(v)
	}
	if err := writeWAV(path, r, y); err != nil {
		t.Fatal(err)
	}
}

func rmsDB(x []float64) float64 {
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return powDB(sum / float64(len(x)))
}

func TestActiveMonoAndLevel(t *testing.T) {
	l, r := make([]float64, rate/2), make([]float64, rate/2)
	click(l, 0.1, 0.8, 0.001, 0.002)
	click(r, 0.1, 0.4, 0.001, 0.002)
	// A fragment 40 dB below the clicks, after a quiet span, is not part
	// of the active span.
	click(l, 0.4, 0.008, 0.001, 0.002)
	x := activeMono(&wav.Audio{SampleRate: rate, Channels: [][]float64{l, r}})
	if n := float64(len(x)) / rate; n < 0.005 || n > 0.05 {
		t.Fatalf("active span of %.3f s, want the click alone", n)
	}
	lo := 0
	for math.Abs(l[lo]) <= dbAmp(defaultFloorDB) && math.Abs(r[lo]) <= dbAmp(defaultFloorDB) {
		lo++
	}
	if want := 0.5 * (l[lo+44] + r[lo+44]); x[44] != want {
		t.Errorf("sample %g, want the mean %g of the channels", x[44], want)
	}
	y := levelled(x, listenRMSDB)
	z := make([]float64, len(y))
	for i, v := range y {
		z[i] = float64(v)
	}
	if got := rmsDB(z); math.Abs(got-listenRMSDB) > 0.01 {
		t.Errorf("RMS level %.2f dBFS, want %d", got, listenRMSDB)
	}
}

func TestListen(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for n := range listenShown + blindTrials {
		x := make([]float64, rate/2)
		for i := range 2 + n%5 {
			click(x, 0.02+0.04*float64(i), 0.8, 0.001, 0.002)
		}
		p := filepath.Join(dir, "c"+string(rune('a'+n))+".wav")
		writeTestWAV(t, p, rate, x)
		paths = append(paths, p)
	}
	out := filepath.Join(dir, "out")
	if err := run(append([]string{"--listen", out}, paths...), io.Discard); err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join(out, "README.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "   0.000  ca.wav") || !strings.Contains(string(readme), "model train, seed 9003") {
		t.Errorf("README.txt has no start of the first clip or of the last model train:\n%s", readme)
	}
	key, err := os.ReadFile(filepath.Join(out, "blind", ".key", "answers.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(key)), "\n")
	if len(lines) != blindTrials {
		t.Fatalf("%d lines in the key, want %d", len(lines), blindTrials)
	}
	var first int
	gap := int(trialGapS * rate)
	for i, line := range lines {
		// The line is "trial-NN: X  (A: name, B: name)", and the clip is
		// item X.
		clipA := line[len("trial-01: ")] == 'A'
		names := strings.Split(strings.TrimSuffix(line[strings.Index(line, "(A: ")+4:], ")"), ", B: ")
		clipName := names[1]
		if clipA {
			first++
			clipName = names[0]
		}
		c, err := wav.ReadFile(filepath.Join(dir, clipName))
		if err != nil {
			t.Fatal(err)
		}
		a, err := wav.ReadFile(filepath.Join(out, "blind", line[:len("trial-01")]+".wav"))
		if err != nil {
			t.Fatal(err)
		}
		// The trial is item A, the first run of gap zeros, and item B, at
		// the same RMS level. The clip keeps the length of its active span.
		x := a.Channels[0]
		lo := -1
		for j, run := 0, 0; j < len(x) && lo < 0; j++ {
			if run++; x[j] != 0 {
				run = 0
			}
			if run == gap {
				lo = j + 1 - gap
			}
		}
		if lo < 0 {
			t.Fatalf("trial %d: no gap of %d samples", i+1, gap)
		}
		itemA, itemB := x[:lo], x[lo+gap:]
		for _, it := range [][]float64{itemA, itemB} {
			if got := rmsDB(it); math.Abs(got-listenRMSDB) > 0.1 {
				t.Errorf("trial %d: item at %.2f dBFS, want %d", i+1, got, listenRMSDB)
			}
		}
		clipItem := itemB
		if clipA {
			clipItem = itemA
		}
		if want := len(activeMono(c)); len(clipItem) != want {
			t.Errorf("trial %d: the clip %s has %d samples, want %d", i+1, clipName, len(clipItem), want)
		}
	}
	if first != blindTrials/2 {
		t.Errorf("the clip is item A in %d of %d trials, want half", first, blindTrials)
	}
	if err := run([]string{"--listen", out, paths[0]}, io.Discard); err == nil {
		t.Error("listen accepted too few clips")
	}
}

func TestJoinFiles(t *testing.T) {
	dir := t.TempDir()
	const r = 48000
	var paths []string
	for i, amp := range []float64{0.5, 0.05} {
		x := make([]float64, 2*r)
		for j := range x {
			x[j] = amp * math.Sin(2*math.Pi*1000*float64(j)/r)
		}
		p := filepath.Join(dir, "r"+string(rune('0'+i))+".wav")
		writeTestWAV(t, p, r, x)
		paths = append(paths, p)
	}
	out := filepath.Join(dir, "joined.wav")
	if err := run(append([]string{"--join", out, "--lufs", "-16"}, paths...), io.Discard); err != nil {
		t.Fatal(err)
	}
	a, err := wav.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	x := a.Channels[0]
	if len(x) != 5*r {
		t.Fatalf("%d samples, want %d", len(x), 5*r)
	}
	for i, part := range [][]float64{x[:2*r], x[3*r:]} {
		if got := analysis.IntegratedLoudness([][]float64{part}, r); math.Abs(got+16) > 0.1 {
			t.Errorf("part %d at %.2f LUFS, want -16", i, got)
		}
	}
	for _, v := range x[2*r : 3*r] {
		if v != 0 {
			t.Fatal("the gap is not silent")
		}
	}
}
