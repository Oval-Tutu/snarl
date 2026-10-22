package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

const (
	// listenRMSDB is the RMS level in dBFS over the active span of each
	// clip and model train of the listening files.
	listenRMSDB = -20
	// listenShown is the number of clips and of model trains of
	// clips-then-model.wav, and blindTrials the number of trials of the
	// blind test, which takes the clips after the shown clips.
	listenShown = 3
	blindTrials = 20
	// listenSeed is the seed of the first model train of the listening
	// files, apart from the seeds of the fit and of the check.
	listenSeed = 9001
	// blindOrderSeed is the seed of the order of the two items of each
	// trial.
	blindOrderSeed = 20261022
	// itemGapS is the silence between two items of clips-then-model.wav
	// and between two inputs of --join, and trialGapS the silence between
	// the two items of a trial.
	itemGapS  = 1
	trialGapS = 0.75
)

const blindReadme = `Blind test of the crackle model: %d trials, trial-01.wav to trial-%02d.wav.

Each trial is item A, %.2f s of silence, then item B. One item is a real
clip of the muffler crackle pack. The other item is a model train of the
crackle kind (internal/crackle). Both items are mono and at an RMS level
of %d dBFS over their active span.

For each trial, write A or B for the real recording.
Open the key in .key/answers.txt only at the end.
14 or more correct of 20 means that the difference is audible (p < 0.06 for chance).
13 or fewer correct means that the model passes.
`

// item is one clip or model train of the listening files, mono and cut to
// its active span.
type item struct {
	name string
	x    []float64
}

// activeMono returns the mono mean of the channels of a over its active
// span (activeSpan), or nil when a has no sound.
func activeMono(a *wav.Audio) []float64 {
	active, _ := activeSpan(a, dbAmp(defaultFloorDB))
	if len(active) == 0 {
		return nil
	}
	return a.Mono()[active[0].lo:active[len(active)-1].hi]
}

// levelled returns x at the RMS level db in dBFS over all of x.
func levelled(x []float64, db float64) []float32 {
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	g := dbAmp(db - powDB(sum/float64(len(x))))
	y := make([]float32, len(x))
	for i, v := range x {
		y[i] = float32(v * g)
	}
	return y
}

// joined returns the parts with gap samples of silence between them.
func joined(parts [][]float32, gap int) []float32 {
	var y []float32
	for i, p := range parts {
		if i > 0 {
			y = append(y, make([]float32, gap)...)
		}
		y = append(y, p...)
	}
	return y
}

// listen writes to dir the listening files of the clips and of model
// trains of the crackle kind: clips-then-model.wav with README.txt, and the
// blind test in blind/ with its key in blind/.key/answers.txt. Each item
// is mono, cut to its active span and at the RMS level listenRMSDB.
func listen(dir string, paths []string) error {
	need := listenShown + blindTrials
	if len(paths) < need {
		return fmt.Errorf("--listen needs %d clips, got %d", need, len(paths))
	}
	clips := make([]item, need)
	peaks := make([]float64, need)
	for i, path := range paths[:need] {
		a, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		if a.SampleRate != modelRate {
			return fmt.Errorf("%s: sample rate %d Hz, want the %d Hz of the model trains", path, a.SampleRate, modelRate)
		}
		x := activeMono(a)
		if x == nil {
			return fmt.Errorf("%s: the clip has no sound", path)
		}
		for _, v := range x {
			peaks[i] = max(peaks[i], math.Abs(v))
		}
		peaks[i] = ampDB(peaks[i])
		clips[i] = item{filepath.Base(path), x}
	}
	p, err := crackle.Fitted()
	if err != nil {
		return err
	}
	seeds := seedRange(listenSeed, need)
	_, _, trains, err := renderModel(p, seeds, median(peaks))
	if err != nil {
		return err
	}
	model := make([]item, need)
	for i, y := range trains {
		x := make([]float64, len(y))
		for j, v := range y {
			x[j] = float64(v)
		}
		model[i] = item{fmt.Sprintf("model train, seed %d", seeds[i]), activeMono(&wav.Audio{SampleRate: modelRate, Channels: [][]float64{x}})}
	}

	if err := os.MkdirAll(filepath.Join(dir, "blind", ".key"), 0o750); err != nil {
		return err
	}
	var readme strings.Builder
	fmt.Fprintf(&readme, "clips-then-model.wav: %d clips of the muffler crackle pack, then %d model trains\nof the crackle kind (internal/crackle). Each item is mono (the mean of the\nchannels), cut to its active span and at an RMS level of %d dBFS over it,\nwith %d s of silence between the items.\n\nStart, s  Source\n", listenShown, listenShown, listenRMSDB, itemGapS)
	var parts [][]float32
	at := 0
	for _, it := range append(append([]item(nil), clips[:listenShown]...), model[:listenShown]...) {
		fmt.Fprintf(&readme, "%8.3f  %s\n", float64(at)/modelRate, it.name)
		parts = append(parts, levelled(it.x, listenRMSDB))
		at += len(it.x) + itemGapS*modelRate
	}
	if err := writeWAV(filepath.Join(dir, "clips-then-model.wav"), modelRate, joined(parts, itemGapS*modelRate)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte(readme.String()), 0o600); err != nil {
		return err
	}

	// The clip is item A in half of the trials, so that a listener who
	// always gives one answer gets half of them right.
	order := rand.New(rand.NewPCG(blindOrderSeed, 0)).Perm(blindTrials) //nolint:gosec // A fixed order, not a secret.
	var key strings.Builder
	for i := range blindTrials {
		a, b, clipAt := clips[listenShown+i], model[listenShown+i], "A"
		if order[i] >= blindTrials/2 {
			a, b, clipAt = b, a, "B"
		}
		fmt.Fprintf(&key, "trial-%02d: %s  (A: %s, B: %s)\n", i+1, clipAt, a.name, b.name)
		y := joined([][]float32{levelled(a.x, listenRMSDB), levelled(b.x, listenRMSDB)}, int(trialGapS*modelRate))
		if err := writeWAV(filepath.Join(dir, "blind", fmt.Sprintf("trial-%02d.wav", i+1)), modelRate, y); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "blind", ".key", "answers.txt"), []byte(key.String()), 0o600); err != nil {
		return err
	}
	text := fmt.Sprintf(blindReadme, blindTrials, blindTrials, trialGapS, listenRMSDB)
	return os.WriteFile(filepath.Join(dir, "blind", "README.txt"), []byte(text), 0o600)
}

// joinFiles writes to out the mono inputs, each at the integrated
// loudness lufs, with itemGapS of silence between them.
func joinFiles(out string, lufs float64, paths []string) error {
	var parts [][]float32
	rate := 0
	for _, path := range paths {
		a, err := wav.ReadFile(path)
		if err != nil {
			return err
		}
		if len(a.Channels) != 1 || (rate != 0 && a.SampleRate != rate) {
			return fmt.Errorf("%s: %d channels at %d Hz, want mono at the rate of the first input", path, len(a.Channels), a.SampleRate)
		}
		rate = a.SampleRate
		x := a.Channels[0]
		l := analysis.IntegratedLoudness(a.Channels, rate)
		if math.IsInf(l, -1) {
			return fmt.Errorf("%s: the input has no loudness above the gate", path)
		}
		analysis.Scale(x, lufs-l)
		y := make([]float32, len(x))
		for i, v := range x {
			y[i] = float32(v)
		}
		parts = append(parts, y)
	}
	if len(parts) == 0 {
		return errors.New("--join needs one or more WAV files")
	}
	return writeWAV(out, rate, joined(parts, itemGapS*rate))
}
