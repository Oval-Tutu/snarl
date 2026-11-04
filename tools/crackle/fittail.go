package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/crackle"
	"github.com/oval-tutu/snarl/internal/wav"
)

// tailKeys are the keys of the parameter file that --fit-tail sets.
var tailKeys = []string{"hold_ms", "trail_shocks", "trail_gap_ms", "trail_level_db", "short_body_db"}

// modelBoom returns the options of model boom i: 1 + subs shocks, with 3
// to 15 later sub-events, as the boom shape of internal/core/pops.go.
func modelBoom(i int) crackle.Options { return crackle.Options{Shocks: 4 + i%13, Short: true} }

// fitTail sets the keys of the tail of the parameter file at path from the
// uncut ends of the short clips (docs/research/pop-tail.md), and writes the
// file and the sources of the keys in the sources file beside it. The
// other keys stay. The hold is the median break of the full band after the
// end event, the trailing shocks are the shocks in the hold (holdShocks),
// and short_body_db is the level of the loudest frame of the model booms
// over that of the model trains.
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
