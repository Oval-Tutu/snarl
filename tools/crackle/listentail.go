package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/oval-tutu/snarl/internal/analysis"
	"github.com/oval-tutu/snarl/internal/core"
	"github.com/oval-tutu/snarl/internal/wav"
)

// tailNames are the two tails of pop-tail-old-then-new.wav, in the order
// of the file: the short tail of the crackle model, then the default.
var tailNames = [2]string{"old tail", "new tail"}

// popTailListen writes to dir pop-tail-old-then-new.wav and README.txt:
// for each case, the cut of variantFromS to variantToS with the pops of
// the short tail (core.UseShortPopTail), then the same cut with the pops
// of the default tail. Each case has the one gain that puts its cut with
// no pops at variantLUFS in both halves.
func popTailListen(dir string, args []string) error {
	cs, err := parseCases(args)
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return errors.New("--pop-tail-listen needs one or more PROFILE=SCENARIO")
	}
	// cuts[i][0] is case i with no pops, and cuts[i][1+t] with tail t.
	cuts := make([][1 + len(tailNames)][]float64, len(cs))
	errs := make([]error, len(cs)*(1+len(tailNames)))
	// New reads the tail when it makes an engine, so the renders of the
	// short tail run first, then the others.
	for _, short := range []bool{true, false} {
		core.UseShortPopTail(short)
		var wg sync.WaitGroup
		sem := make(chan struct{}, runtime.GOMAXPROCS(0))
		for i, c := range cs {
			for j := range 1 + len(tailNames) {
				if (j == 1) != short {
					continue
				}
				wg.Go(func() {
					sem <- struct{}{}
					defer func() { <-sem }()
					x, _, err := renderEngine(c, levelSets, j == 0, variantRate)
					if err == nil && len(x) < variantToS*variantRate {
						err = fmt.Errorf("%s: the scenario is shorter than %d s", c.scenario, variantToS)
					}
					if err != nil {
						errs[i*(1+len(tailNames))+j] = err
						return
					}
					cuts[i][j] = x[variantFromS*variantRate : variantToS*variantRate]
				})
			}
		}
		wg.Wait()
	}
	core.UseShortPopTail(false)
	if err := errors.Join(errs...); err != nil {
		return err
	}
	var readme strings.Builder
	fmt.Fprintf(&readme, "pop-tail-old-then-new.wav: the pops of %d engines with the old tail, then\nthe new tail (docs/research/pop-tail.md), made by just listen-pop-tail. For\neach engine, 2 lift-offs (%d s to %d s of the lift-off scenario) with the old\ntail, then the same 2 lift-offs with the new tail, the race pop character\nat the default pop level, mono at %d Hz. Each engine has the gain that puts\nit with no pops at %d LUFS, the same gain in both halves. The silence between\ntwo parts is %d s, and between two engines %d s.\n\nStart, s  Engine, tail\n",
		len(cs), variantFromS, variantToS, variantRate, variantLUFS, itemGapS, sectionGapS)
	var y []float32
	gap := func(s int) { y = append(y, make([]float32, s*variantRate)...) }
	for i, c := range cs {
		l := analysis.IntegratedLoudness([][]float64{cuts[i][0]}, variantRate)
		if math.IsInf(l, -1) {
			return fmt.Errorf("%s: the cut with no pops has no loudness above the gate", c.profile)
		}
		if i > 0 {
			gap(sectionGapS)
		}
		for t, name := range tailNames {
			if t > 0 {
				gap(itemGapS)
			}
			fmt.Fprintf(&readme, "%8.3f  %s, %s\n", float64(len(y))/variantRate, c.profile, name)
			y = append(y, faded(cuts[i][1+t], variantLUFS-l, int(fadeS*variantRate))...)
		}
	}
	fmt.Fprintf(&readme, "%8.3f  end\n", float64(len(y))/variantRate)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := writeWAV(filepath.Join(dir, "pop-tail-old-then-new.wav"), variantRate, y); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(readme.String()), 0o600)
}

// boomToS ends the boom part of pop-tail-current-then-fixed.wav: one
// lift-off, from variantFromS.
const boomToS = variantFromS + (variantToS-variantFromS)/2

// cutNames are the cuts of each case that popTailFixed saves: the cut with
// no pops, with the pops, and the boom part with only the boom in use.
var cutNames = [3]string{"nopops", "pops", "boom"}

// popTailFixed renders the cuts of cutNames of each case with the pops of
// the default tail and writes them to dir/cuts as PROFILE-CUT.wav. With
// before, a directory of the same cuts of an earlier tail, it also writes
// to dir pop-tail-current-then-fixed.wav and README.txt: for each case, the
// cut with the pops of before, then that of dir, then the same for the boom
// part. Each case has the one gain that puts its cut of before with no
// pops at variantLUFS in all four parts.
func popTailFixed(dir, before string, args []string) error {
	cs, err := parseCases(args)
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return errors.New("--pop-tail-fixed needs one or more PROFILE=SCENARIO")
	}
	boom := append(append([]string(nil), levelSets...), "pops.weight_pop=0", "pops.weight_boom=1", "pops.weight_crackle=0", "pops.weight_burble=0")
	cuts := make([][len(cutNames)][]float64, len(cs))
	errs := make([]error, len(cs)*len(cutNames))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, c := range cs {
		for j := range cutNames {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				sets, to := levelSets, variantToS
				if j == 2 {
					sets, to = boom, boomToS
				}
				x, _, err := renderEngine(c, sets, j == 0, variantRate)
				if err == nil && len(x) < to*variantRate {
					err = fmt.Errorf("%s: the scenario is shorter than %d s", c.scenario, to)
				}
				if err != nil {
					errs[i*len(cutNames)+j] = err
					return
				}
				cuts[i][j] = x[variantFromS*variantRate : to*variantRate]
			})
		}
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "cuts"), 0o750); err != nil {
		return err
	}
	for i, c := range cs {
		for j, n := range cutNames {
			y := make([]float32, len(cuts[i][j]))
			for k, v := range cuts[i][j] {
				y[k] = float32(v)
			}
			if err := writeWAV(cutPath(dir, c.profile, n), variantRate, y); err != nil {
				return err
			}
		}
	}
	if before == "" {
		return nil
	}
	var readme strings.Builder
	fmt.Fprintf(&readme, "pop-tail-current-then-fixed.wav: the pops of %d engines with the current tail,\nthen the fixed tail (docs/research/pop-tail.md section 9). For each engine, 2\nlift-offs (%d s to %d s of the lift-off scenario) with the current tail, then\nthe same 2 lift-offs with the fixed tail, the race pop character at the\ndefault pop level, mono at %d Hz. The boom is rare in the lift-offs: with the\nweights of the profiles, 2 in 5 first pops of a lift-off run and 1 in 5 later\npops are booms. So each engine then plays a boom part: the first lift-off (%d s to %d s) with only the\nboom in use (pops.weight_boom = 1, the other weights 0), with the current tail,\nthen with the fixed tail. Each engine has the gain that puts it with no pops\nat %d LUFS, the same gain in all four parts. The silence between two parts is\n%d s, and between two engines %d s.\n\nStart, s  Engine, part\n",
		len(cs), variantFromS, variantToS, variantRate, variantFromS, boomToS, variantLUFS, itemGapS, sectionGapS)
	var y []float32
	gap := func(s int) { y = append(y, make([]float32, s*variantRate)...) }
	for i, c := range cs {
		a, err := readCut(cutPath(before, c.profile, cutNames[0]))
		if err != nil {
			return err
		}
		l := analysis.IntegratedLoudness([][]float64{a}, variantRate)
		if math.IsInf(l, -1) {
			return fmt.Errorf("%s: the cut with no pops has no loudness above the gate", c.profile)
		}
		if i > 0 {
			gap(sectionGapS)
		}
		for k, part := range []struct{ cut, name string }{{"pops", "current tail"}, {"pops", "fixed tail"}, {"boom", "boom part, current tail"}, {"boom", "boom part, fixed tail"}} {
			from := before
			if k%2 == 1 {
				from = dir
			}
			x, err := readCut(cutPath(from, c.profile, part.cut))
			if err != nil {
				return err
			}
			if k > 0 {
				gap(itemGapS)
			}
			fmt.Fprintf(&readme, "%8.3f  %s, %s\n", float64(len(y))/variantRate, c.profile, part.name)
			y = append(y, faded(x, variantLUFS-l, int(fadeS*variantRate))...)
		}
	}
	fmt.Fprintf(&readme, "%8.3f  end\n", float64(len(y))/variantRate)
	if err := writeWAV(filepath.Join(dir, "pop-tail-current-then-fixed.wav"), variantRate, y); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(readme.String()), 0o600)
}

func cutPath(dir, profile, cut string) string {
	return filepath.Join(dir, "cuts", profile+"-"+cut+".wav")
}

// readCut reads a mono cut of popTailFixed at variantRate.
func readCut(path string) ([]float64, error) {
	a, err := wav.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if a.SampleRate != variantRate || len(a.Channels) != 1 {
		return nil, fmt.Errorf("%s: want mono at %d Hz", path, variantRate)
	}
	return a.Channels[0], nil
}
