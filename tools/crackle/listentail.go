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
