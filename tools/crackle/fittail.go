package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

// tailKeys are the keys of the parameter file that --fit-tail sets.
var tailKeys = []string{"hold_ms", "trail_shocks", "trail_gap_ms", "trail_level_db", "short_body_db", "hold_tail_db"}

const (
	// lowBand and lowDrop are the band below 200 Hz of decayBands and the
	// fall to -20 dB of decayDrops, which hold_tail_db sets.
	lowBand = 1
	lowDrop = 1
	// holdTailMinDB and holdTailSteps are the range and the steps of the
	// bisection of hold_tail_db.
	holdTailMinDB = -60.0
	holdTailSteps = 16
)

// fitHoldTail returns the gain of the tail of a single shock from
// holdTailMinDB to 0 dB at which the median time of singleLow is want.
func fitHoldTail(p *crackle.Params, want float64) float64 {
	lo, hi := float64(holdTailMinDB), 0.0
	for range holdTailSteps {
		mid := (lo + hi) / 2
		if singleLow(p, mid) > want {
			hi = mid
		} else {
			lo = mid
		}
	}
	return (lo + hi) / 2
}

// singleLow renders tailSeeds model single shocks with the tail gain db
// from the seed checkSeed, and returns the median over their end events of
// the time to lowDrop in lowBand.
func singleLow(p *crackle.Params, db float64) float64 {
	c := *p
	c.HoldTailDB = db
	bank := crackle.NewBank(&c, modelRate, checkSeed)
	ms := make([]float64, tailSeeds)
	var wg sync.WaitGroup
	for i, seed := range seedRange(checkSeed, tailSeeds) {
		wg.Go(func() {
			ms[i] = math.NaN()
			y := crackle.Render(bank, seed, crackle.Options{Single: true})
			x := make([]float64, len(y)+int(padS*modelRate))
			for j, v := range y {
				x[j] = float64(v)
			}
			if s := newTailSeg(x, modelRate); s != nil {
				if r, _, cut := s.endEvent(modelRate); cut == uncut {
					ms[i] = r[lowBand].DropMS[lowDrop]
				}
			}
		})
	}
	wg.Wait()
	return median(finite(ms))
}

// modelBoom returns the options of model boom i: 1 + subs shocks, with 3
// to 15 later sub-events, as the boom shape of internal/core/pops.go.
func modelBoom(i int) crackle.Options { return crackle.Options{Shocks: 4 + i%13, Short: true} }

// fitTail sets the keys of the tail of the parameter file at path from the
// uncut ends of the short clips (docs/research/pop-tail.md), and writes the
// file and the sources of the keys in the sources file beside it. The
// other keys stay. The hold is the median break of the full band after the
// end event, the trailing shocks are the shocks in the hold (holdShocks),
// hold_tail_db is the gain of the tail of the model single shocks that
// gives their band below 200 Hz the median fall of the clip ends
// (fitHoldTail), and short_body_db is the level of the loudest frame of
// the model booms over that of the model trains.
func fitTail(path string, clips []string) error {
	data, err := os.ReadFile(path) //nolint:gosec // The path is a user-supplied parameter file.
	if err != nil {
		return err
	}
	p, err := crackle.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	t := &tailCollector{groups: map[string]*tailGroup{}}
	for _, c := range clips {
		a, err := wav.ReadFile(c)
		if err != nil {
			return err
		}
		if s := newTailSeg(a.Mono(), a.SampleRate); s != nil {
			t.add("clips", filepath.Base(c), s, a.SampleRate, false)
		}
	}
	g := t.groups["clips|end"]
	if g == nil || len(g.rows) == 0 {
		return errors.New("--fit-tail: no clip has an uncut end")
	}
	p.HoldMS = median(decayColumn(g.rows, 0, func(d decay) float64 { return d.BreakMS }))
	counts, gaps, levels := holdStats(g.rows)
	p.TrailShocks, p.TrailGapMS, p.TrailLevelDB = quantiles(counts), quantiles(gaps), quantiles(levels)
	low := median(decayColumn(g.rows, lowBand, func(d decay) float64 { return d.DropMS[lowDrop] }))
	p.HoldTailDB = fitHoldTail(p, low)
	p.ShortBodyDB = 0
	boom, train := loudest(p, modelBoom), loudest(p, func(int) crackle.Options { return crackle.Options{} })
	p.ShortBodyDB = boom - train
	if err := os.WriteFile(path, p.Marshal(paramsHeader), 0o600); err != nil {
		return err
	}
	ends := fmt.Sprintf("the %d uncut ends of the %d short clips (tools/crackle --pop-tail, docs/research/pop-tail.md)", len(g.rows), len(clips))
	src := []source{
		{"hold_ms", "measured", "the median break of the two lines through the full-band envelope after the end event, the last strong shock, of " + ends},
		{"trail_shocks", "measured", fmt.Sprintf("the quantiles at the probabilities (i + 0.5) / 21 of the count of the shocks in the hold after the end event, from the end event to the break, of %s", ends)},
		{"trail_gap_ms", "measured", fmt.Sprintf("the quantiles at the probabilities (i + 0.5) / 21 of the gaps from the onset of the shock before of the %d shocks in the hold of %s, pooled", len(gaps), ends)},
		{"trail_level_db", "measured", fmt.Sprintf("the quantiles at the probabilities (i + 0.5) / 21 of the level against the strongest shock of the clip of the %d shocks in the hold of %s, pooled", len(levels), ends)},
		{"hold_tail_db", "derived", fmt.Sprintf("set by bisection from %g to 0 dB so that the median time to -%g dB %s after the end event of %d model single shocks is %.1f ms, the median of %s, against %.1f ms at 0 dB; the damped sine of the tail starts from rest at the fade, and its sound from 0 to %.0f Hz holds the band below 200 Hz up after the fade", holdTailMinDB, decayDrops[lowDrop], decayBands[lowBand].name, tailSeeds, low, ends, singleLow(p, 0), p.TailHz)},
		{"short_body_db", "derived", fmt.Sprintf("the level of the loudest %g ms frame of %d model booms (4 to 16 shocks, Options.Short), median, %.1f dB, less that of %d model trains, %.1f dB, whose bed and body the fit sets against the loudest frame of the clips (bed_db, body_gain_db), so that a boom has the bed and the body of the clips against its loudest frame; no clip holds a short train", frameS*1000, tailSeeds, boom, tailSeeds, train)},
	}
	return writeTailSources(sourcesPath(path), src)
}

// loudest renders tailSeeds model trains of the options of opt from the
// seed checkSeed and returns the median over them of the level of their
// loudest frame, the peak of the RMS envelope over frameS, in dB.
func loudest(p *crackle.Params, opt func(i int) crackle.Options) float64 {
	bank := crackle.NewBank(p, modelRate, checkSeed)
	db := make([]float64, tailSeeds)
	var wg sync.WaitGroup
	for i, seed := range seedRange(checkSeed, tailSeeds) {
		wg.Go(func() {
			y := crackle.Render(bank, seed, opt(i))
			x := make([]float64, len(y))
			for j, v := range y {
				x[j] = float64(v)
			}
			db[i] = ampDB(slices.Max(rmsEnv(x, modelRate, frameS)))
		})
	}
	wg.Wait()
	return median(db)
}

// sourcesPath returns the sources file beside the parameter file.
func sourcesPath(path string) string {
	dir, name := filepath.Split(path)
	return filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".sources.toml")
}

// writeTailSources replaces the entries of the keys of src in the sources
// file, and the header, so that the file keeps the entries of the fit.
func writeTailSources(path string, src []source) error {
	data, err := os.ReadFile(path) //nolint:gosec // The path is beside a user-supplied parameter file.
	if err != nil {
		return err
	}
	parts := strings.Split(string(data), "\n[[source]]\n")
	var b strings.Builder
	b.WriteString(sourcesHeader)
	for _, e := range parts[1:] {
		key, _, _ := strings.Cut(strings.TrimPrefix(e, "key = "), "\n")
		if !slices.Contains(tailKeys, strings.Trim(key, `"`)) {
			b.WriteString("\n[[source]]\n" + strings.TrimRight(e, "\n") + "\n")
		}
	}
	for _, s := range src {
		fmt.Fprintf(&b, "\n[[source]]\nkey = %q\nkind = %q\nfrom = %q\n", s.key, s.kind, s.from)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
