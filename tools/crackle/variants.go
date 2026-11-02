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
)

const (
	// variantRate is the rate of the level variants, as in just
	// listen-pops-model.
	variantRate = 48000
	// variantFromS and variantToS cut two lift-offs from each render of
	// the lift-off scenarios: the second and third full-throttle holds and
	// the overrun after each. The first hold heats the exhaust from cold.
	variantFromS = 6
	variantToS   = 18
	// variantLUFS is the integrated loudness of each engine with no pops.
	variantLUFS = -19
	// sectionGapS is the silence between two variants, and fadeS the fade
	// at the ends of each engine part.
	sectionGapS = 3
	fadeS       = 0.01
	// defaultLU is popOffsetLU of internal/core: the loudness of the pops
	// over the engine against the level of the RMS calibration.
	defaultLU = -10
)

// variantLU holds the pop loudness of the variants against the level of
// the RMS calibration, in equal steps of 4 LU from 2 LU below it.
var variantLU = [4]float64{-2, -6, -10, -14}

// popVariants writes to dir pop-level-variants.wav and README.txt: for each
// variant of variantLU, the cut of each case with its pops at the variant
// level through the profile pop gain. Each case has the one gain that puts
// its cut with no pops at variantLUFS, in every variant, so that only the
// pops change between the variants.
func popVariants(dir string, args []string) error {
	cs, err := parseCases(args)
	if err != nil {
		return err
	}
	if len(cs) == 0 {
		return errors.New("--pop-variants needs one or more PROFILE=SCENARIO")
	}
	// cuts[i][0] is case i with no pops, and cuts[i][1+v] with variant v.
	cuts := make([][1 + len(variantLU)][]float64, len(cs))
	errs := make([]error, len(cs)*(1+len(variantLU)))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, c := range cs {
		for j := range 1 + len(variantLU) {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				sets := append([]string(nil), levelSets...)
				if j > 0 {
					sets = append(sets, fmt.Sprintf("pops.gain=%.6f", dbAmp(variantLU[j-1]-defaultLU)))
				}
				x, _, err := renderEngine(c, sets, j == 0, variantRate)
				if err == nil && len(x) < variantToS*variantRate {
					err = fmt.Errorf("%s: the scenario is shorter than %d s", c.scenario, variantToS)
				}
				if err != nil {
					errs[i*(1+len(variantLU))+j] = err
					return
				}
				cuts[i][j] = x[variantFromS*variantRate : variantToS*variantRate]
			})
		}
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	gains := make([]float64, len(cs))
	for i := range cs {
		l := analysis.IntegratedLoudness([][]float64{cuts[i][0]}, variantRate)
		if math.IsInf(l, -1) {
			return fmt.Errorf("%s: the cut with no pops has no loudness above the gate", cs[i].profile)
		}
		gains[i] = variantLUFS - l
	}
	var readme strings.Builder
	fmt.Fprintf(&readme, "pop-level-variants.wav: %d variants of the pop level over the engine, made by\njust listen-pop-level. Each variant plays %d engines, with 2 lift-offs each\n(%d s to %d s of the lift-off scenario), the race pop character, mono at %d Hz.\nEach engine has the gain that puts it with no pops at %d LUFS, the same gain\nin every variant, so that only the pops change. The pop level is in LU\nagainst the level before the loudness calibration. The default is variant B\n(%d LU) until the choice. The silence between two engines is %d s, and\nbetween two variants %d s.\n\nStart, s  Variant, engine\n",
		len(variantLU), len(cs), variantFromS, variantToS, variantRate, variantLUFS, defaultLU, itemGapS, sectionGapS)
	var y []float32
	gap := func(s int) { y = append(y, make([]float32, s*variantRate)...) }
	for v, lu := range variantLU {
		if v > 0 {
			gap(sectionGapS)
		}
		fmt.Fprintf(&readme, "%8.3f  %c: pops at %+g LU\n", float64(len(y))/variantRate, 'A'+v, lu)
		for i, c := range cs {
			if i > 0 {
				gap(itemGapS)
			}
			fmt.Fprintf(&readme, "%8.3f     %s\n", float64(len(y))/variantRate, c.profile)
			y = append(y, faded(cuts[i][1+v], gains[i], int(fadeS*variantRate))...)
		}
	}
	fmt.Fprintf(&readme, "%8.3f  end\n", float64(len(y))/variantRate)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := writeWAV(filepath.Join(dir, "pop-level-variants.wav"), variantRate, y); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.txt"), []byte(readme.String()), 0o600)
}

// faded returns x at the gain in dB, with linear fades of n samples at both
// ends.
func faded(x []float64, gainDB float64, n int) []float32 {
	g := dbAmp(gainDB)
	y := make([]float32, len(x))
	for i, v := range x {
		f := min(1, float64(min(i, len(x)-1-i))/float64(n))
		y[i] = float32(v * g * f)
	}
	return y
}
