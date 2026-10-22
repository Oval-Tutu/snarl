package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

// TestCheck renders model trains, writes them and checks them against
// clips, and checks that the loss of a set against itself is 0.
func TestCheck(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := range 3 {
		y := crackle.Render(crackle.NewBank(crackle.Default(), modelRate, 1), uint32(100+i), crackle.Options{})
		p := filepath.Join(dir, fmt.Sprintf("clip%d.wav", i))
		if err := writeWAV(p, y); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	out := filepath.Join(dir, "model")
	var stdout bytes.Buffer
	if err := run(append([]string{"--model", "2", "--render", out}, paths...), &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "# Crackle model against the clips") {
		t.Error("the check has no title")
	}
	for i := range 2 {
		if _, err := os.Stat(filepath.Join(out, fmt.Sprintf("crackle_%d.wav", i+1))); err != nil {
			t.Error(err)
		}
	}
	var clips []*clip
	for _, p := range paths {
		a, err := wav.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		c, err := measure(p, a, dbAmp(defaultFloorDB))
		if err != nil {
			t.Fatal(err)
		}
		clips = append(clips, c)
	}
	if l := loss(clips, clips); l != 0 {
		t.Errorf("loss %.3f of the clips against themselves, want 0", l)
	}
}

// TestFitBank checks that the bed bank, fitted to band powers, gives them
// in the measure bands.
func TestFitBank(t *testing.T) {
	p := crackle.Default()
	p.TopHz = 12000
	var target [crackle.Bands]float64
	for b := lowBands; b < crackle.Bands; b++ {
		target[b] = math.Pow(10, (-20-3*float64(b))/10)
	}
	target[len(bands)-1] = math.Pow(10, -70.0/10)
	h := bankMatrix(p.TopHz)
	g := fitBank(h, target)
	p.BedDB = g
	for b := range p.ShockDB {
		p.ShockDB[b], p.RingDB[b] = offDB, offDB
	}
	p.BodyGainDB = offDB
	y := crackle.Render(crackle.NewBank(p, modelRate, 1), 7, crackle.Options{LengthMS: 600})
	x := make([]float64, len(y))
	for i, v := range y {
		x[i] = float64(v) / dbAmp(p.OutDB)
	}
	x = x[modelRate/10 : modelRate/2]
	for b := lowBands; b < len(bands); b++ {
		e := bands[b]
		y := analysis.BandPass(x, modelRate, e[0], e[1])[modelRate/20:]
		got := powDB(meanSquare(y))
		want := powDB(target[b])
		if b == len(bands)-1 {
			// The leak of the band below sets the floor of the top band.
			if got > -50 {
				t.Errorf("band %s at %.1f dB, want below -50 dB", bandName(b), got)
			}
			continue
		}
		if math.Abs(got-want) > 1 {
			t.Errorf("band %s at %.1f dB, want %.1f dB", bandName(b), got, want)
		}
	}
}
