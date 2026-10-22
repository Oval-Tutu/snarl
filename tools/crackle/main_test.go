package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/wav"
)

const rate = 44100

// click adds to x at t seconds a 3 kHz tone of amplitude a whose envelope
// rises linearly over rise seconds and then decays with the time constant
// tau.
func click(x []float64, t, a, rise, tau float64) {
	i0 := int(t * rate)
	for i := i0; i < len(x); i++ {
		s := float64(i-i0) / rate
		env := math.Exp(-(s - rise) / tau)
		if s < rise {
			env = s / rise
		}
		if env < 1e-6 && s > rise {
			break
		}
		x[i] += a * env * math.Sin(2*math.Pi*3000*s)
	}
}

func mono(x []float64) *wav.Audio {
	return &wav.Audio{SampleRate: rate, Channels: [][]float64{x}}
}

func TestFindShocks(t *testing.T) {
	x := make([]float64, rate/5)
	times := []float64{0.02, 0.05, 0.09}
	levels := []float64{1, 0.5, 0.25}
	for i, tt := range times {
		click(x, tt, levels[i], 0.002, 0.001)
	}
	hp := analysis.BandPass(x, rate, shockHPHz, shockLPHz)
	s := findShocks(x, hp, rate)
	if len(s) != 3 {
		t.Fatalf("found %d shocks, want 3", len(s))
	}
	for i, sh := range s {
		if d := sh.T - times[i]; d < -0.0005 || d > 0.001 {
			t.Errorf("shock %d at %.4f s, want near %.4f s", i, sh.T, times[i])
		}
		if want := ampDB(levels[i]); math.Abs(sh.RelDB-want) > 1 {
			t.Errorf("shock %d at %.1f dB, want %.1f dB", i, sh.RelDB, want)
		}
		// The envelope rises over 2 ms, so 10 to 90 per cent takes about
		// 1.6 ms, and the 0.25 ms envelope smears it a little.
		if sh.RiseMS < 1.3 || sh.RiseMS > 1.9 {
			t.Errorf("shock %d rises in %.2f ms, want about 1.6 ms", i, sh.RiseMS)
		}
	}
	if !math.IsNaN(s[0].Gap) || math.Abs(s[1].Gap-0.03) > 0.0005 || math.Abs(s[2].Gap-0.04) > 0.0005 {
		t.Errorf("gaps %v, %v and %v, want NaN, 0.03 and 0.04", s[0].Gap, s[1].Gap, s[2].Gap)
	}
	if !s[0].Strong || !s[1].Strong || s[2].RelDB < -strongDB != !s[2].Strong {
		t.Errorf("strong flags %v %v %v", s[0].Strong, s[1].Strong, s[2].Strong)
	}
}

func TestChannels(t *testing.T) {
	l := make([]float64, rate/2)
	for i, tt := range []float64{0.05, 0.12, 0.2, 0.31} {
		click(l, tt, 1/float64(i+1), 0.0005, 0.003)
	}
	const lag = 10
	r := make([]float64, len(l))
	for i := lag; i < len(r); i++ {
		r[i] = 0.5 * l[i-lag]
	}
	c, err := measure("stereo", &wav.Audio{SampleRate: rate, Channels: [][]float64{l, r}}, dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	if want := lag * 1000.0 / rate; math.Abs(c.DelayMS-want) > 0.005 {
		t.Errorf("delay %.4f ms, want %.4f ms", c.DelayMS, want)
	}
	if math.Abs(c.LevelDiffDB-ampDB(2)) > 0.01 {
		t.Errorf("level difference %.3f dB, want 6.02 dB", c.LevelDiffDB)
	}
	if c.LagCorr < 0.999 || c.Corr >= c.LagCorr {
		t.Errorf("correlation %.4f at no delay and %.4f at the delay", c.Corr, c.LagCorr)
	}
}

func TestActiveSpan(t *testing.T) {
	x := make([]float64, rate)
	fill := func(t0, t1, a float64) {
		for i := int(t0 * rate); i < int(t1*rate); i++ {
			x[i] = a * math.Sin(2*math.Pi*100*float64(i)/rate)
		}
	}
	fill(0.1, 0.3, 1)
	fill(0.33, 0.43, 0.5)
	fill(0.6, 0.61, 0.003)
	c, err := measure("span", mono(x), dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	if c.Bursts != 2 || math.Abs(c.QuietMS[0]-30) > 0.5 {
		t.Errorf("%d bursts with quiet spans %v ms, want 2 with 30 ms", c.Bursts, c.QuietMS)
	}
	if math.Abs(c.Start-0.1) > 1e-3 || math.Abs(c.Length-0.33) > 1e-3 {
		t.Errorf("active span from %.4f s for %.4f s, want from 0.1 s for 0.33 s", c.Start, c.Length)
	}
	if math.Abs(c.LateMS-600) > 0.5 || math.Abs(c.LateDB-ampDB(0.003)) > 0.1 {
		t.Errorf("late sound at %.1f ms and %.1f dBFS, want 600 ms and %.1f dBFS", c.LateMS, c.LateDB, ampDB(0.003))
	}
}

func TestTail(t *testing.T) {
	x := make([]float64, rate*4/10)
	click(x, 0.01, 1, 0.0005, 0.001)
	// The body falls 20 dB in 80 ms.
	tau := 0.08 / math.Ln10
	for i := int(0.02 * rate); i < len(x); i++ {
		s := float64(i)/rate - 0.02
		x[i] += 0.5 * math.Exp(-s/tau) * math.Sin(2*math.Pi*50*s)
	}
	c, err := measure("tail", mono(x), dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	tl := c.Tail
	if !tl.Ok {
		t.Fatalf("no tail, %d shocks", len(c.Shocks))
	}
	if math.Abs(tl.CycleHz-50) > 1 || math.Abs(tl.Hz-50) > 2 || math.Abs(c.BodyHz-50) > 2 {
		t.Errorf("tail at %.1f Hz from the half cycles and %.1f Hz from the spectrum, body at %.1f Hz, want 50 Hz", tl.CycleHz, tl.Hz, c.BodyHz)
	}
	if math.Abs(tl.Decay20MS-80) > 4 || tl.R2 < 0.99 {
		t.Errorf("decay to -20 dB in %.1f ms with r squared %.3f, want 80 ms", tl.Decay20MS, tl.R2)
	}
}

func TestBandShares(t *testing.T) {
	x := make([]float64, rate/2)
	for i := range x {
		x[i] = math.Sin(2 * math.Pi * 3000 * float64(i) / rate)
	}
	c, err := measure("tone", mono(x), dbAmp(defaultFloorDB))
	if err != nil {
		t.Fatal(err)
	}
	if c.BandDB[hiBand] < -1 || c.HiDB < -0.5 || c.LowDB > -50 {
		t.Errorf("2 to 4 kHz share %.2f dB, above 2 kHz %.2f dB, below 125 Hz %.1f dB", c.BandDB[hiBand], c.HiDB, c.LowDB)
	}
	if bands[hiBand][0] != 2000 || bands[lowBands][0] != 125 {
		t.Errorf("hiBand starts at %v Hz and lowBands ends at %v Hz", bands[hiBand][0], bands[lowBands][0])
	}
}

func TestKMeans(t *testing.T) {
	var x [][]float64
	for i := range 10 {
		d := float64(i%3) * 0.1
		x = append(x, []float64{d, -d}, []float64{10 + d, 10 - d})
	}
	a := kmeans(x, 2, newRand())
	for i := 2; i < len(x); i++ {
		if a[i] != a[i%2] {
			t.Fatalf("point %d is in class %d, want %d", i, a[i], a[i%2])
		}
	}
	if a[0] == a[1] {
		t.Fatal("the two groups share a class")
	}
	if s := silhouette(x, a, 2); s < 0.95 {
		t.Errorf("silhouette %.3f, want near 1", s)
	}
}

func TestSubtractAndSplit(t *testing.T) {
	bed := make([]float64, rate*2)
	for i := range bed {
		bed[i] = 0.3 * math.Sin(2*math.Pi*80*float64(i)/rate)
	}
	x := append([]float64(nil), bed...)
	click(x, 0.2, 0.5, 0.0005, 0.005)
	click(x, 1.2, 0.5, 0.0005, 0.005)
	a := mono(x)
	if err := subtract(a, mono(bed)); err != nil {
		t.Fatal(err)
	}
	parts := splitEvents(a, 0.3, dbAmp(defaultFloorDB))
	if len(parts) != 2 {
		t.Fatalf("%d events, want 2", len(parts))
	}
	if err := subtract(a, mono(bed[:rate])); err == nil {
		t.Error("subtract accepted a reference of another length")
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for n := range 4 {
		x := make([]float32, rate/2)
		y := make([]float64, len(x))
		for i := range 1 + 2*n {
			click(y, 0.02+0.03*float64(i), 0.8, 0.001, 0.002)
		}
		for i, v := range y {
			x[i] = float32(v)
		}
		p := filepath.Join(dir, "c"+string(rune('0'+n))+".wav")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		w, err := wav.NewWriter(f, rate, wav.F32, len(x))
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Write(x); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	out := filepath.Join(dir, "out")
	var stdout bytes.Buffer
	if err := run(append([]string{"--out", out, "--k", "2"}, paths...), &stdout); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Classes", "## Clip measures", "## Strong shocks", "## Band energy over time, class B", "## Spectra, class A"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("summary has no %q", want)
		}
	}
	for _, name := range []string{"clips.csv", "shocks.csv", "bands.csv"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(data), "\n"); lines < 5 {
			t.Errorf("%s has %d lines", name, lines)
		}
	}
	if err := run([]string{"--k", "-1", paths[0]}, &stdout); err == nil {
		t.Error("run accepted a negative k")
	}
}
