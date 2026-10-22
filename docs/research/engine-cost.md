# Engine cost: profile and byte-identical savings

Measured on the branch `upstream-profiles` after `c5e89e9`, on an AMD Ryzen 9 7940HS, one core, Go 1.27.2 for amd64, at 48 kHz in 256-frame blocks. The engine is the cycle table player (`internal/cycles`, `Live`) into the synthesiser runtime (`internal/esynth`, `Runtime`), with the game features of `internal/core`. Sections 1 to 7 describe the engine at `c5e89e9`, with the pop side input. Section 8 gives the pop cost after the crackle model of the branch `pop-model`, which removes the side input.

## Answer

The residual noise of the table player was the largest stage: 19 to 45 per cent of the engine. Its band filters ran per simulation step with their states in memory. `Live` now computes the band noise of 64 steps at a time, with the same draws in the same order and each band cascade in one loop with its states in registers. The output does not change: all 100 scorecard renders (five engines, ten scenarios, with and without the own pops) and two grid renders are byte-identical to `c5e89e9`, and the scores are the same to the last digit.

The engine now costs 0.47 to 0.85 per cent of one core here in `BenchmarkRender` and the four states, against 0.52 to 1.03 per cent before: 7 to 24 per cent less. While pops ring, it costs 0.72 to 0.98 per cent, against 0.85 to 1.20. The target of 0.30 to 0.40 per cent per engine is not reached. The synthesiser runtime alone (input, main chain, convolution, resampler) is 0.32 to 0.42 per cent, so no change to the table player alone can reach the target. The runtime is close to its floor for scalar Go with the same arithmetic: the changes that I tried there saved 5 per cent of a stage or less, so I did not keep them.

Recommendation: when the target must hold on every engine, merge the two exhaust channels before the synthesiser (section 5, option 1). It saves about 0.09 per cent per engine and moves every score by 0.7 or less. I did not apply it, because it changes the output. Measure the new binary on the phone first (section 6): the per cent of this PC are an estimate of the phone cost.

## 1. Profile before the changes

`go test -cpuprofile` on `BenchmarkStates` (the four states of one engine, 2 s each) and on `BenchmarkPops`, with `go tool pprof -top -cum -focus='core.\(\*Engine\).Render'`, so that `New` and the pop calibration do not count. The share of each stage is the share of `Engine.Render`. The per cent of one core is the share times the median of the four states.

| Stage | Functions | `i4-vtec` | `v8-ls` | `f4-ej25` | `v12-412` | `v12-merlin` |
| --- | --- | --- | --- | --- | --- | --- |
| Table playback | `Live.Next` less the residual: the weights, the intake lag, the speed profile, the wave tables | 17.5 % (0.17) | 8.2 % (0.06) | 15.2 % (0.15) | 6.2 % (0.03) | 7.8 % (0.05) |
| Residual noise | `Live.noise`: the band filters, the draws, the envelopes | 44.6 % (0.42) | 30.3 % (0.22) | 42.5 % (0.41) | 19.1 % (0.10) | 24.1 % (0.14) |
| Main synthesiser chain | `WriteInputSide` (interpolation, 1900 Hz filter), `pre` (draws, air noise, jitter, DC, derivative, mix), output filter and leveller | 20.6 % (0.20) | 29.9 % (0.22) | 23.8 % (0.23) | 36.8 % (0.20) | 32.8 % (0.19) |
| Convolution | `Convolver.Process` | 10.7 % (0.10) | 20.1 % (0.15) | 10.0 % (0.10) | 25.5 % (0.14) | 23.8 % (0.14) |
| Resampler | `Resampler.Pull` | 5.1 % (0.05) | 6.5 % (0.05) | 6.1 % (0.06) | 8.6 % (0.05) | 8.3 % (0.05) |
| Limiter, pops, fade | `Engine.Render` and `feed` themselves, `control`, `limit`, the pops | 1.7 % (0.02) | 4.9 % (0.04) | 2.4 % (0.02) | 3.9 % (0.02) | 3.3 % (0.02) |
| Engine | median of the four states | 0.949 | 0.729 | 0.968 | 0.545 | 0.582 |

The value in brackets is the per cent of one core. The pop side chain does not run in the four states: `preSide` has no samples. In `BenchmarkPops` (overrun with a pop request every 64 ms, all five engines), the side chain is about 23 per cent of the engine: `preSide` 6.6 per cent, the side half of the input stage 3.5 per cent, and the side convolution about 13 per cent. The pops themselves (`pops.tick`, `ringBody`) are below 2 per cent.

The table playback and the residual noise run per simulation step, so their cost rises with the simulation rate: 20 kHz for `i4-vtec` and `f4-ej25`, 10 kHz for `v8-ls`, 7 kHz for `v12-merlin` and 5 kHz for `v12-412`. The synthesiser runs per 44.1 kHz sample and costs about the same on every engine.

The pop side chain already runs only while a pop or its tail rings: `core.feed` writes the side input only while a pop is active or `hold` counts down the impulse response, and `WriteInputSide` with a nil side stops it. Nothing was left to gain there with the same output.

## 2. Changes

The changes are in `internal/cycles` only. The runtime, the dsp package and the core are unchanged.

| Change | File | Why the output is the same |
| --- | --- | --- |
| `Live.fillNoise` computes the band noise of the next 64 steps into `bandNoise`, and `Next` reads one value per band and channel. | `internal/cycles/live.go` | The draws and the band filters do not depend on the input of `Next`. `fillNoise` draws in the order of the steps, the channels and the bands, which is the order of the per-step code. Each filter value is the same arithmetic. |
| `band.block` runs a cascade of two or four sections in one loop, with the states in local variables and the coefficients read from memory. | `internal/cycles/bands.go` | The input history of a section is the output history of the section before it, so that the states are the same values. Each section computes the expression of `biquad.process`. |
| `Live.noise` sums the envelope of each band with the weights in the outer loop, and loads the noise row of a point once per weight instead of once per band. | `internal/cycles/live.go` | Each band envelope sums the weights in their order from 0, and the residual sums the bands in their order, as before. |
| `BenchmarkPops`: overrun at four fifths of the redline with a pop request every 12 blocks, so that the pop side chain runs in every block. | `bench_test.go` | A benchmark only. It is the peak cost of the engine. |
| `TestBandBlock` and `TestLiveResidual` check the block filters and the residual of `Live` bit for bit against the per-step filters. | `internal/cycles/live_test.go` | Tests only. |

Tried and not kept, because each saved less than its added code was worth or was slower:

- A channel pair of band filters in one loop (`block2`). It was as fast as the cascade loop, which needs no equal coefficients across channels.
- The convolution sum with four bins in registers over all partitions. It was 5 to 10 per cent slower than the loop over partitions.
- The 1900 Hz input filter states in local variables. No measurable change: the calls write 2 to 9 samples each.
- The resampler sum with two loop steps per pass. 5 per cent of a 2.4 us stage, about 0.002 per cent of one core.

The convolution, the FFT and the resampler are bound by the float32 operations themselves. With the same arithmetic, the next step down is SIMD (assembly for amd64 and arm64), which I did not try.

## 3. Sound check

`go run ./tools/scorecard --scenarios holds-t000,holds-t050,holds-t100,sweep-wot,overrun,simrate,simrate-2x,idle-free,idle-free-rpm,limiter-free`, once with the defaults and once with `--dyno --no-pops`, at `c5e89e9` (`out/cost/base`, `out/cost/base-np`) and after the changes (`out/cost/final`, `out/cost/final-np`). `diff -rq` finds no difference in the 100 renders, and `cmp` finds no difference in the score tables. The grid scenarios are not in the scorecard: `snarl-render -format f32` of `i4-vtec/grid/step.scn` and `v12-merlin/grid/t025.scn` is byte-identical too (`out/cost/grid-base`, `out/cost/grid-after`).

| Engine | Renders compared | Byte-identical | Score change |
| --- | --- | --- | --- |
| `i4-vtec` | 20 scorecard, 1 grid | all | 0.0 |
| `v8-ls` | 20 scorecard | all | 0.0 |
| `f4-ej25` | 20 scorecard | all | 0.0 |
| `v12-412` | 20 scorecard | all | 0.0 |
| `v12-merlin` | 20 scorecard, 1 grid | all | 0.0 |

The scores with `--dyno --no-pops` stay 97.7 to 100 on the holds, the sweeps, the overrun, the simulation-rate hold and the free-running limiter.

The check is on amd64. On arm64, Go can fuse a multiplication and an addition into one instruction. The new loops keep the expressions of the old code, so that the same fusions apply, but I did not compare arm64 renders.

## 4. Benchmarks

`BenchmarkRender`, `BenchmarkStates` and `BenchmarkPops`, `-test.benchtime=0.5s -test.count=4`, run four times in turn: the `c5e89e9` binary, the new binary, the `c5e89e9` binary, the new binary. Each value is the median of 8 counts, in per cent of one core at 48 kHz. The clock of this PC drifts by up to 10 per cent between runs, so a change below 5 per cent is noise. Every count allocates 0 bytes.

| Engine | Benchmark | Before | After | Change |
| --- | --- | --- | --- | --- |
| `i4-vtec` | Render | 1.029 | 0.829 | -19 % |
| `i4-vtec` | wot | 0.987 | 0.753 | -24 % |
| `i4-vtec` | idle | 0.934 | 0.739 | -21 % |
| `i4-vtec` | overrun | 0.997 | 0.815 | -18 % |
| `i4-vtec` | limiter | 0.877 | 0.738 | -16 % |
| `i4-vtec` | pops | 1.145 | 0.984 | -14 % |
| `v8-ls` | Render | 0.766 | 0.659 | -14 % |
| `v8-ls` | wot | 0.710 | 0.625 | -12 % |
| `v8-ls` | idle | 0.701 | 0.624 | -11 % |
| `v8-ls` | overrun | 0.770 | 0.653 | -15 % |
| `v8-ls` | limiter | 0.733 | 0.641 | -13 % |
| `v8-ls` | pops | 1.023 | 0.922 | -10 % |
| `f4-ej25` | Render | 1.026 | 0.851 | -17 % |
| `f4-ej25` | wot | 0.951 | 0.783 | -18 % |
| `f4-ej25` | idle | 0.932 | 0.746 | -20 % |
| `f4-ej25` | overrun | 0.981 | 0.801 | -18 % |
| `f4-ej25` | limiter | 1.006 | 0.776 | -23 % |
| `f4-ej25` | pops | 1.197 | 0.958 | -20 % |
| `v12-412` | Render | 0.541 | 0.496 | -8 % |
| `v12-412` | wot | 0.520 | 0.475 | -9 % |
| `v12-412` | idle | 0.544 | 0.466 | -14 % |
| `v12-412` | overrun | 0.565 | 0.494 | -13 % |
| `v12-412` | limiter | 0.550 | 0.480 | -13 % |
| `v12-412` | pops | 0.852 | 0.719 | -16 % |
| `v12-merlin` | Render | 0.612 | 0.549 | -10 % |
| `v12-merlin` | wot | 0.558 | 0.503 | -10 % |
| `v12-merlin` | idle | 0.579 | 0.535 | -8 % |
| `v12-merlin` | overrun | 0.608 | 0.557 | -8 % |
| `v12-merlin` | limiter | 0.584 | 0.541 | -7 % |
| `v12-merlin` | pops | 0.916 | 0.794 | -13 % |

The profile after the changes, in the form of section 1:

| Stage | `i4-vtec` | `v8-ls` | `f4-ej25` | `v12-412` | `v12-merlin` |
| --- | --- | --- | --- | --- | --- |
| Table playback | 15.8 % (0.12) | 10.4 % (0.07) | 19.0 % (0.15) | 6.5 % (0.03) | 8.7 % (0.05) |
| Residual noise | 34.8 % (0.26) | 20.3 % (0.13) | 37.0 % (0.29) | 12.8 % (0.06) | 16.9 % (0.09) |
| Main synthesiser chain | 26.6 % (0.20) | 35.5 % (0.23) | 27.0 % (0.21) | 36.3 % (0.17) | 35.0 % (0.19) |
| Convolution | 12.5 % (0.10) | 21.7 % (0.14) | 8.0 % (0.06) | 28.7 % (0.14) | 26.3 % (0.14) |
| Resampler | 6.8 % (0.05) | 8.3 % (0.05) | 6.4 % (0.05) | 10.2 % (0.05) | 8.2 % (0.04) |
| Limiter, pops, fade | 3.6 % (0.03) | 3.8 % (0.02) | 2.6 % (0.02) | 5.6 % (0.03) | 4.9 % (0.03) |
| Engine | 0.761 | 0.636 | 0.777 | 0.479 | 0.534 |

The residual noise is now 0.06 to 0.29 per cent, against 0.10 to 0.42 before. The runtime stages (main chain, convolution, resampler) are 0.32 to 0.42 per cent, which matches the 0.38 per cent of the runtime alone in `esynth-runtime.md` section 4.

## 5. Options that change the output

Each option was applied on its own to the new code, rendered with the scorecard and timed, then removed. None is in the tree. The score change is against the scores of section 3 with `--dyno --no-pops`, except for option 4, which needs the own pops. The CPU saving is per engine, in per cent of one core here, from back-to-back runs of the same benchmark.

| Option | CPU saving | Score change | Sound change |
| --- | --- | --- | --- |
| 1. Merge the two exhaust channels before the synthesiser: one runtime channel takes the sum of the two table channels. All five engines have one impulse response and one volume for both channels, so the convolution sum is the same. | 0.09 (`i4-vtec` wot 42.8 to 37.5 us per block, `v12-412` 27.8 to 23.1 us) | -0.5 to +0.7 on every scenario, 0.1 or less on every hold, sweep and overrun | One set of jitter and air noise draws instead of one per channel. The cost of the input stage and `pre` halves. |
| 2. Shorten the impulse response to 4096 taps (`RuntimeOptions.MaxTaps`) | 0.04 on `v8-ls`, `v12-412` and `v12-merlin` (runtime 20.2 to 18.1 us per block), none on `i4-vtec` and `f4-ej25` (1893 taps) | -0.3 to +0.3, except `v8-ls` idle-free -3.3 | Up to 0.92 dB of third-octave distance (`esynth-runtime.md` section 3) |
| 3. A 16-tap resampler (`RuntimeOptions.ResamplerTaps`) | about 0.02, half of the 2.4 us stage (an estimate from the tap count) | -0.1 to +0.2 | A wider transition band near 20 kHz. Not measured against aliasing. |
| 4. Sum the pop side input into the main convolution, so that the leveller sees the pops. It no longer applies: the side input is gone (section 8). | 0.08 to 0.23 while a pop rings (`BenchmarkPops` -7 to -21 %), none otherwise | With the own pops: overrun -1.3 to +0.9, all else unchanged | The pops duck the engine through the leveller again, which the side input removed (`TestPopLeveller`). |
| 5. Second-order residual bands instead of fourth-order | about 0.1 on `i4-vtec`, none measurable on `v12-412` | holds -0.4 to -2.1, idle-free-rpm down to -8.6 (`v12-merlin`) | The band shapes no longer match the measured envelopes. Not recommended. |
| 6. No residual noise | 0.06 to 0.29 | holds down to 57.8 (`v12-412` holds-t000), simrate 75.6 | Rejected: the residual is most of the score on `v12-412`. |

Recommendation: option 1 first. With options 1, 2 and 3 together, the estimate is about 0.33 to 0.38 per cent for `v12-412` and `v12-merlin`, 0.49 for `v8-ls`, and 0.65 to 0.67 for `i4-vtec` and `f4-ej25`. The estimate adds the savings of the options, which I did not measure together. The two 20 kHz engines stay above the target, because their residual noise alone is 0.26 to 0.29 per cent. A cheaper residual for those two engines needs a new design, for example the band noise at half the simulation rate for the lower bands, and a new scorecard run.

## 6. Phone

`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c -o out/tmp/snarl-bench-arm64 .` builds the root package with `BenchmarkRender`, `BenchmarkStates` and `BenchmarkPops`. `out/tmp/phone-bench.txt` holds the `adb` commands that run it on cpu0 (`taskset 1`). With the factors of `esynth-runtime.md` (0.38 per cent here is about 2.5 per cent of a OnePlus 3T slow core and 3.9 to 5.2 per cent of a Switch core), the new engine is about 3.1 to 5.6 per cent of the 3T slow core and 4.8 to 11.6 per cent of a Switch core, and more while pops ring. Measure it on the phone: this is an estimate.

## 7. How to run

```sh
go test -c -o out/cost/after.test .
./out/cost/after.test -test.run=NONE -test.bench='BenchmarkRender|BenchmarkStates|BenchmarkPops' -test.benchtime=0.5s -test.count=4
./out/cost/after.test -test.run=NONE -test.bench='BenchmarkStates/i4-vtec/' -test.benchtime=2s -test.cpuprofile=out/cost/after-i4-vtec.prof
go tool pprof -top -cum -nodefraction=0 -focus='core.\(\*Engine\).Render' out/cost/after.test out/cost/after-i4-vtec.prof
go run ./tools/scorecard --dyno --no-pops --out out/cost/final-np
```

## 8. Pops after the crackle model

On the branch `pop-model`, each pop plays on a voice of the crackle model (`internal/crackle`). `Engine.mix` adds the voices to each runtime block after the leveller, the volume and the clamp, and before the resampler (`esynth.RuntimeOptions.Mix`). The pop side input and its second runtime pass are gone: `WriteInputSide`, `preSide` and the side convolution. The pop side chain of section 1 and option 4 of section 5 no longer apply.

| Benchmark | With the side input | With the crackle model |
| --- | --- | --- |
| `BenchmarkPops` on this PC, mean of the five engines, per cent of one core | 0.97 | 0.78 |
| `BenchmarkPops` on the phone slow core (cpu0), medians of 8 counts, per cent of one core | 5.6 to 6.7 (`out/phone/phone-pops.txt`) | 4.0 to 5.7 (`out/phone/pop-model-pops.txt`) |

On this PC, a pop now costs about 0.09 per cent of one core over the engine, against 0.28 per cent with the side input ([crackle-model.md](crackle-model.md) section 9.6). With pops off, the output is byte-identical to the engine before the change.
