# esynth runtime: the engine-sim synthesiser for the audio goroutine

Measured on the branch `upstream-profiles` after `1b5be2e`, on an AMD Ryzen 9 7940HS, one core, Go 1.27.2 for amd64. `esynth.Runtime` (`internal/esynth/runtime.go`) is a real-time mode of the exact port in `internal/esynth` (`docs/research/esynth-port.md`). The exact mode is unchanged, and its null test passes.

## Answer

The runtime renders one 256-frame block at 48 kHz in 20.4 us for a two-channel engine with the longest impulse response (6907 taps), which is 0.38 per cent of one core. A one-channel engine costs 0.31 per cent. The exact port costs 2.5 ms for the same block (47 per cent of one core), so the runtime is about 120 times faster. With the factors of the earlier estimate (0.30 per cent here is about 2 per cent of a OnePlus 3T slow core and 3.1 to 4.1 per cent of a Switch core), 0.38 per cent here is about 2.5 per cent of the 3T and 3.9 to 5.2 per cent of the Switch. Measure it on the phone: this is an estimate.

The runtime output agrees with the engine-sim output on all 20 renders (five engines, hold and sweep, noise off and on). At 44.1 kHz the peak difference is 3.4 to 4.6 16-bit steps, 73.1 dB or more below the signal peak, and the RMS difference is 83.6 dB or more below the signal RMS. At 48 kHz, against the engine-sim output resampled by an independent resampler, the peak difference is 75.4 dB or more below the peak. The worst third-octave distance of any window is 0.059 dB, the worst low-band third-octave distance 0.075 dB, and the alignment scorecard is 100.0 on every engine.

Recommendation: keep the full impulse responses. The FFT convolution makes the length cheap: 4096 taps instead of 6907 saves 0.036 per cent of one core (9 per cent of the runtime) and costs up to 0.92 dB of third-octave distance. 2048 and 1024 taps change the sound clearly. Use a convolution block of 256 samples and the 32-tap resampler from 44.1 to 48 kHz.

## 1. Design

### 1.1 Interface

```go
p := esynth.SimulatorParameters(channels, simulationFrequency)
r, err := esynth.NewRuntime(p, irs, volumes, esynth.RuntimeOptions{OutputSampleRate: 48000}, esynth.NewRandom(1))
r.SetAudioParameters(audio)       // the engine's Synthesizer::AudioParameters
r.SetInputSampleRate(rate)        // per frame, as Simulator::startFrame does
r.WriteInput(stepSamples)         // one sample per channel and simulation step
n := r.Render(out)                // on the audio goroutine; n < len(out) when the input runs out
```

`NewRuntime` takes the same parameters, impulse responses and volumes as `New` and `InitializeImpulseResponse`. `RuntimeOptions` sets the output rate, the convolution block (`Block`, default 256), a shorter impulse response (`MaxTaps`, `FadeTaps`), the resampler length (`ResamplerTaps`, default 32) and the input buffer (`InputBuffer`, default 8192 samples at 44.1 kHz, 186 ms). `Render` and `WriteInput` do not allocate or lock, and they do not divide or call `math.Sin`, `math.Exp` or `math.Pow` per sample. `TestRuntimeNoAlloc` checks the allocations. `SetAudioParameters` designs the air noise filter again only when its cutoff changes, and then it allocates.

The latency of `Render` is one convolution block (256 samples, 5.8 ms) and the look-ahead of the resampler (16 samples, 0.4 ms). Output sample i at 44.1 kHz is the output of input sample i, as in the exact port. At 48 kHz, output sample j is at time j * 44100 / 48000 of the 44.1 kHz stream.

### 1.2 Stages

The runtime keeps the stages, constants and random numbers of the port. It changes the arithmetic only.

| Stage | Exact port | Runtime |
| --- | --- | --- |
| Input interpolation | float64 offsets, `t / distance` per sample | the same offsets in float64, so that the samples fall at the same times. The division is a multiplication by the reciprocal of the step. |
| 1900 Hz input filter | fourth-order direct form in double | two second-order sections in double. In float32 the difference from engine-sim grows from 4 to 11 steps on the `v12-merlin` sweep, so the filter stays in double, as engine-sim computes it. |
| Input buffer | ring buffer of 44100 floats per channel | ring buffer of 8192 float32 per channel. A full buffer drops new samples and counts them (`Overflows`). |
| `rand()` draws | one call per draw | `Random.fill` runs the glibc recurrence x[n] = x[n-31] + x[n-3] over a block: the same values. `TestRandomFill` checks it. |
| Jitter | `minstd_rand0` with the modulus, the 10 kHz filter per sample, `floor` and `ceil` | the modulus folds the high bits, which gives the same draws (`TestUniformFast`). The filter runs over the block. The floor and the ceiling are integer conversions. |
| Air noise filter, output filter | fourth-order direct form in float, `f^4 / a0` per sample | the same filter in float32 with `f^4 / a0` stored, run over the block with its state in registers. The older outputs are summed first, which shortens the dependency from one output to the next. |
| DC filter, derivative | `dt / (rc + dt)` and `/ dt` per sample | stored coefficient and reciprocal |
| Convolution | direct form over a circular shift register | uniformly partitioned overlap-save FFT convolution in float32 (`dsp.Convolver`), block 256, FFT length 512. Channels with the same impulse response and volume share one convolution of their sum, which is the same sum. All five engines have two channels with one impulse response. |
| Leveller | `target / peak` per sample | the reciprocal of the peak: the decay multiplies it by 1/0.999, a new peak computes it by three Newton steps (`TestReciprocal`). A peak below the smallest normal float is 0. |
| Output | `lround`, clamp to int16 | the same clamp, no rounding, float32 from -1 to 1 |
| 48 kHz | none | polyphase Kaiser-windowed sinc resampler (`dsp.Resampler`): 160 phases of 32 taps, -6 dB at 20.9 kHz, beta 8 |

`dsp.RealFFT` is an iterative radix-2 FFT of real blocks over complex64. The products are written out in float32, because the Go `*` operator widens complex64 to float64 for a product. That change made the convolution 30 per cent faster.

### 1.3 44.1 kHz with a resampler, not 48 kHz

The runtime runs the synthesiser at 44.1 kHz and resamples to 48 kHz. The resampler costs 2.6 us per block, 13 per cent of the runtime. Running the synthesiser at 48 kHz instead costs 48/44.1 of the other stages, 1.6 us more per block, so the resampler costs about 1 us more than a 48 kHz synthesiser. A 48 kHz synthesiser changes the sound: the impulse responses are 44.1 kHz recordings, the jitter moves over 10 samples, and the leveller decays by 0.999 per sample. The resampler keeps the sound of engine-sim for 1 us per block, so the runtime uses it.

For a sine from 100 Hz to 16 kHz, the error of the resampler is 85 dB or more below the sine (`TestResampler`). On the 20 renders it agrees with `analysis.Resample`, an independent 64-tap Blackman resampler, within 3.7 steps (section 2.2).

## 2. Equivalence

Tolerance: the peak difference from the engine-sim output 60 dB or more below the signal peak, the RMS difference 70 dB or more below the signal RMS, and a third-octave distance below 0.1 dB in every window. `TestRuntimeEquivalence` and `TestRuntimeEquivalence48k` check the sample tolerances on the committed `i4-vtec` excerpts. On the excerpts the peak difference is 76.4 dB or more below the peak at 44.1 kHz and 78.4 dB or more at 48 kHz.

The reference is the engine-sim output in the record, which the `Float32` port reproduces bit for bit. The difference is over the whole output, the pre-roll included, in 16-bit steps. A runtime sample differs when it is more than half a step from the engine-sim sample.

### 2.1 44.1 kHz

| Engine | Scenario | Noise | Peak difference (dB below peak) | RMS difference (dB below RMS) |
| --- | --- | --- | --- | --- |
| `v8-ls` | holds | on | 3.64 (75.8) | 0.559 (83.6) |
| `v8-ls` | holds | off | 3.76 (75.6) | 0.560 (83.7) |
| `v8-ls` | sweep | on | 4.11 (74.8) | 0.539 (83.6) |
| `v8-ls` | sweep | off | 3.81 (75.4) | 0.548 (83.6) |
| `i4-vtec` | holds | on | 4.11 (73.7) | 0.607 (83.8) |
| `i4-vtec` | holds | off | 4.07 (73.8) | 0.595 (83.9) |
| `i4-vtec` | sweep | on | 4.54 (73.6) | 0.598 (83.8) |
| `i4-vtec` | sweep | off | 3.47 (75.6) | 0.589 (83.9) |
| `f4-ej25` | holds | on | 3.79 (74.7) | 0.619 (83.8) |
| `f4-ej25` | holds | off | 4.42 (73.1) | 0.615 (83.9) |
| `f4-ej25` | sweep | on | 3.93 (74.3) | 0.621 (83.8) |
| `f4-ej25` | sweep | off | 3.97 (74.1) | 0.625 (83.9) |
| `v12-412` | holds | on | 3.65 (75.7) | 0.533 (83.6) |
| `v12-412` | holds | off | 4.28 (74.3) | 0.547 (83.6) |
| `v12-412` | sweep | on | 3.45 (76.1) | 0.543 (83.6) |
| `v12-412` | sweep | off | 3.56 (75.9) | 0.554 (83.6) |
| `v12-merlin` | holds | on | 3.48 (77.7) | 0.541 (83.7) |
| `v12-merlin` | holds | off | 3.85 (75.5) | 0.605 (84.1) |
| `v12-merlin` | sweep | on | 3.45 (77.4) | 0.524 (83.7) |
| `v12-merlin` | sweep | off | 4.59 (73.6) | 0.598 (84.2) |

The runtime is as close to engine-sim as the `Float64` port (3 or 4 steps, 74.0 dB or more below the peak): both differ by the float rounding of engine-sim.

### 2.2 48 kHz

The reference is the engine-sim output resampled to 48 kHz by `analysis.Resample`. The last 64 output samples are left out: there the resamplers read past the end of the record, where the runtime holds the tail of its convolution and the reference holds zeros. Against the engine-sim output through the runtime resampler, which leaves the resampler out, the peak difference is 2.7 to 3.4 steps, 75.8 dB or more below the peak.

| Engine | Scenario | Noise | Peak difference (dB below peak) | RMS difference (dB below RMS) |
| --- | --- | --- | --- | --- |
| `v8-ls` | holds | on | 2.95 (77.6) | 0.491 (84.7) |
| `v8-ls` | holds | off | 3.37 (76.6) | 0.492 (84.8) |
| `v8-ls` | sweep | on | 2.69 (78.5) | 0.473 (84.7) |
| `v8-ls` | sweep | off | 3.11 (77.2) | 0.478 (84.8) |
| `i4-vtec` | holds | on | 3.19 (75.9) | 0.510 (85.3) |
| `i4-vtec` | holds | off | 3.09 (76.2) | 0.495 (85.5) |
| `i4-vtec` | sweep | on | 3.41 (76.1) | 0.502 (85.3) |
| `i4-vtec` | sweep | off | 3.09 (76.6) | 0.489 (85.5) |
| `f4-ej25` | holds | on | 3.49 (75.4) | 0.516 (85.3) |
| `f4-ej25` | holds | off | 3.23 (75.8) | 0.509 (85.5) |
| `f4-ej25` | sweep | on | 2.99 (76.7) | 0.516 (85.5) |
| `f4-ej25` | sweep | off | 3.12 (76.2) | 0.517 (85.5) |
| `v12-412` | holds | on | 2.99 (77.5) | 0.484 (84.5) |
| `v12-412` | holds | off | 3.47 (76.1) | 0.494 (84.5) |
| `v12-412` | sweep | on | 3.10 (77.0) | 0.483 (84.6) |
| `v12-412` | sweep | off | 3.06 (77.2) | 0.489 (84.7) |
| `v12-merlin` | holds | on | 3.40 (77.7) | 0.477 (84.8) |
| `v12-merlin` | holds | off | 3.29 (76.9) | 0.493 (85.9) |
| `v12-merlin` | sweep | on | 3.74 (76.8) | 0.472 (84.6) |
| `v12-merlin` | sweep | off | 3.32 (76.4) | 0.482 (86.1) |

### 2.3 Spectral distance

`snarl-compare` compares the engine-sim output (`ref.wav`) with the runtime output of the noise-on renders after the pre-roll. The worst window of each metric over the scenario:

| Engine | Scenario | Third-octave 44.1 / 48 kHz (dB) | Low-band third-octave 44.1 / 48 kHz (dB) | Order spectrum 44.1 / 48 kHz (dB) |
| --- | --- | --- | --- | --- |
| `v8-ls` | holds | 0.047 / 0.046 | 0.075 / 0.075 | 0.001 / 0.001 |
| `v8-ls` | sweep | 0.014 / 0.020 | 0.005 / 0.018 | 0.001 / 0.017 |
| `i4-vtec` | holds | 0.050 / 0.051 | 0.023 / 0.023 | 0.003 / 0.004 |
| `i4-vtec` | sweep | 0.055 / 0.059 | 0.003 / 0.017 | 0.003 / 0.016 |
| `f4-ej25` | holds | 0.010 / 0.011 | 0.010 / 0.010 | 0.002 / 0.003 |
| `f4-ej25` | sweep | 0.008 / 0.012 | 0.003 / 0.009 | 0.002 / 0.009 |
| `v12-412` | holds | 0.015 / 0.015 | 0.025 / 0.024 | 0.001 / 0.001 |
| `v12-412` | sweep | 0.002 / 0.010 | 0.002 / 0.011 | 0.001 / 0.010 |
| `v12-merlin` | holds | 0.001 / 0.005 | 0.001 / 0.005 | 0.002 / 0.015 |
| `v12-merlin` | sweep | 0.001 / 0.014 | 0.001 / 0.014 | 0.001 / 0.038 |

The alignment scorecard (`snarl-compare --align`) of the 44.1 kHz runtime against engine-sim on the holds is 100.0 on all five engines, with 0.00 dB in the residual, burst, envelope and roughness distances.

## 3. Impulse response length

The runtime shortens each impulse response with `--taps N`: it keeps the first N taps after the engine-sim trim and fades the last N/8 with a raised cosine. `mild_exhaust.wav` (`i4-vtec`, `f4-ej25`) has 1893 taps, so only 1024 shortens it. The measures compare engine-sim with the shortened runtime at 44.1 kHz with noise on. The full length gives the values of section 2.3 and a score of 100.0.

| Engine | Taps | Third-octave holds / sweep (dB) | Low-band third-octave holds / sweep (dB) | Order spectrum holds / sweep (dB) | Score | Residual difference (dB) | Burst Mod distance (dB) | Envelope distance (dB) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `v8-ls` | 4096 | 0.46 / 0.49 | 0.75 / 0.78 | 0.93 / 0.45 | 100.0 | 0.20 | 0.08 | 0.47 |
| `v8-ls` | 2048 | 3.62 / 2.63 | 5.76 / 4.17 | 4.20 / 2.11 | 96.2 | 0.79 | 0.49 | 2.03 |
| `v8-ls` | 1024 | 6.59 / 6.01 | 8.99 / 6.86 | 11.13 / 6.93 | 55.8 | 3.88 | 2.20 | 6.46 |
| `v12-412` | 4096 | 0.92 / 0.62 | 1.48 / 1.01 | 0.79 / 0.16 | 100.0 | 0.29 | 0.12 | 0.22 |
| `v12-412` | 2048 | 4.26 / 2.05 | 6.62 / 3.18 | 3.75 / 1.54 | 91.9 | 3.03 | 0.79 | 1.99 |
| `v12-412` | 1024 | 8.31 / 6.99 | 10.51 / 8.90 | 7.01 / 5.96 | 58.6 | 9.19 | 1.84 | 2.51 |
| `v12-merlin` | 4096 | 0.31 / 0.34 | 0.38 / 0.42 | 1.09 / 0.57 | 100.0 | 0.27 | 0.12 | 0.26 |
| `v12-merlin` | 2048 | 1.58 / 2.16 | 2.43 / 2.63 | 2.84 / 3.57 | 99.3 | 0.46 | 1.15 | 1.76 |
| `v12-merlin` | 1024 | 5.02 / 6.14 | 5.25 / 8.20 | 6.42 / 7.92 | 45.9 | 3.76 | 6.09 | 5.71 |
| `i4-vtec` | 1024 | 2.14 / 1.44 | 3.30 / 1.95 | 4.74 / 2.30 | 94.6 | 1.37 | 1.07 | 7.37 |
| `f4-ej25` | 1024 | 1.54 / 1.91 | 2.20 / 2.92 | 3.18 / 2.96 | 97.9 | 1.20 | 1.23 | 3.11 |

The third-octave and order columns are the worst window of the scenario. Score, residual, burst and envelope are from `--align` on the holds: the residual difference, the burst Mod distance and the envelope distance are the worst window, and their calibration floors are 2.00, 1.30 and 1.50 dB. 4096 taps keep the score at 100.0 and the burst within 0.12 dB, but the third-octave distance reaches 0.92 dB on `v12-412` and the low band 1.48 dB. 2048 and 1024 taps break the 1 dB limit on every engine.

The saving per length, at block 256 for two channels with one impulse response (section 4):

| Taps | Per cent of real time | Saving against 6907 taps |
| --- | --- | --- |
| 6907 | 0.382 | none |
| 5979 | 0.380 | 0.002 |
| 4096 | 0.346 | 0.036 (9 per cent) |
| 2048 | 0.337 | 0.045 (12 per cent) |
| 1024 | 0.324 | 0.058 (15 per cent) |

The convolution is a quarter to a half of the runtime cost, and the cost grows slowly with the length. The quality cost of a shorter response is larger than its saving, so the runtime keeps the full length by default.

## 4. Benchmarks

`go test ./internal/esynth/ -run '^$' -bench '^BenchmarkRuntime$' -cpu 1`, median of five runs. Each operation is one 256-frame block at 48 kHz (5.333 ms of sound): `WriteInput` for the simulation steps of the block (20 kHz input of the `i4-vtec` excerpt) and `Render`, with the resampler. The impulse responses are synthetic, because the cost depends on the length only. "2 channels" share one impulse response, as all five engines do. "2 separate" have two different responses and two convolutions. The machine ran other jobs at a load of about 3 during the runs, and repeated runs vary by about 3 per cent.

Block 256 (the default, 5.8 ms latency):

| Taps | 1 channel ns/block | 1 channel % | 2 channels ns/block | 2 channels % | 2 separate ns/block | 2 separate % |
| --- | --- | --- | --- | --- | --- | --- |
| 1024 | 12930 | 0.243 | 17300 | 0.324 | 20350 | 0.382 |
| 1893 | 13010 | 0.244 | 16980 | 0.318 | 21610 | 0.405 |
| 2048 | 13800 | 0.259 | 17960 | 0.337 | 21500 | 0.403 |
| 4096 | 14410 | 0.270 | 18450 | 0.346 | 24590 | 0.461 |
| 5979 | 17100 | 0.321 | 20250 | 0.380 | 27240 | 0.511 |
| 6907 | 16570 | 0.311 | 20390 | 0.382 | 30040 | 0.563 |

Block 128 (2.9 ms latency):

| Taps | 1 channel ns/block | 1 channel % | 2 channels ns/block | 2 channels % | 2 separate ns/block | 2 separate % |
| --- | --- | --- | --- | --- | --- | --- |
| 1024 | 13800 | 0.259 | 18030 | 0.338 | 21460 | 0.402 |
| 1893 | 14530 | 0.272 | 18450 | 0.346 | 24140 | 0.453 |
| 2048 | 15550 | 0.292 | 18640 | 0.350 | 26370 | 0.495 |
| 4096 | 19270 | 0.361 | 23650 | 0.443 | 35240 | 0.661 |
| 5979 | 22380 | 0.420 | 26840 | 0.503 | 39030 | 0.732 |
| 6907 | 23790 | 0.446 | 27310 | 0.512 | 41020 | 0.769 |

Block 256 is 25 per cent cheaper than block 128 with the long responses, and one 256-frame audio block needs at most one convolution block. A noisy earlier run at block 512 gave 0.35 per cent for 6907 taps and two channels, a further 8 per cent, for 11.6 ms of latency.

The cost by stage, two channels with one 6907-tap response, block 256, per 256-frame block (`BenchmarkRuntimeStages`, `BenchmarkPreParts`):

| Stage | ns/block | Share |
| --- | --- | --- |
| Input: interpolation and the 1900 Hz filter (`WriteInput`) | 1930 | 9 per cent |
| Before the convolution (`pre`): `rand()` draws 600, air noise filter 1200, jitter draws 1100, jitter filter 780, jitter, DC removal, derivative and mix 2330 | 6010 | 30 per cent |
| Convolution: 2 FFTs of 512 and 27 partitions | 8330 | 41 per cent |
| After the convolution (`post`): output filter, leveller, volume, clamp | 1380 | 7 per cent |
| Resampler 44.1 to 48 kHz | 2590 | 13 per cent |
| Sum | 20240 | |

The exact port on the same block (`BenchmarkExactPort`, two channels, 44.1 kHz output):

| Precision | Taps | ns/block | Per cent of real time |
| --- | --- | --- | --- |
| `Float64` | 1893 | 859000 | 16.1 |
| `Float64` | 6907 | 2523000 | 47.3 |
| `Float32` | 1893 | 1984000 | 37.2 |
| `Float32` | 6907 | 6350000 | 119 |

For the phone, cross-compile the test binary and run the same benchmark: `GOOS=android GOARCH=arm64 go test -c -o esynth.test ./internal/esynth/`, then `./esynth.test -test.run '^$' -test.bench '^BenchmarkRuntime$/block=256/' -test.cpu 1` in a directory that holds `testdata/esynth/` two levels up. The Go compiler fuses multiply-adds on arm64, which can make the filters and the convolution faster there.

## 5. How to run

```sh
go test ./internal/esynth/ ./internal/dsp/ ./tools/esynth/
go run ./tools/esynth --dump out/esynth/v8-ls/holds-t050-noise --runtime --out out/esynth-runtime/v8-ls/holds-t050-noise/rt44.wav
go run ./tools/esynth --dump out/esynth/v8-ls/holds-t050-noise --runtime --rate 48000 --out out/esynth-runtime/v8-ls/holds-t050-noise/rt48.wav
go run ./tools/esynth --dump out/esynth/v8-ls/holds-t050-noise --runtime --taps 4096 --out out/esynth-runtime/v8-ls/holds-t050-noise/rt44-t4096.wav
go run ./cmd/snarl-compare --scenario scenarios/upstream/v8-ls/holds-t050.scn --spectrograms=false out/esynth/v8-ls/holds-t050-noise/ref.wav out/esynth-runtime/v8-ls/holds-t050-noise/rt44.wav
go run ./cmd/snarl-compare --align --scenario scenarios/upstream/v8-ls/holds-t050.scn out/esynth/v8-ls/holds-t050-noise/ref.wav out/esynth-runtime/v8-ls/holds-t050-noise/rt44.wav
```

`tools/esynth --runtime` prints the difference from the engine-sim output of the record. At another rate than 44.1 kHz it prints two differences: against the engine-sim output through the runtime resampler, and against `analysis.Resample`. The renders, the reports and the benchmark output are under `out/esynth-runtime/`.

## 6. Limits

- In the game, `rand()` has no `CombustionChamber::ignite` draws between the blocks, so the noise and the jitter take other values than in an engine-sim render. Their statistics are the same.
- `Render` renders whole convolution blocks. It needs 256 samples of input at 44.1 kHz beyond the samples that it has rendered, and it returns fewer samples than asked when the input runs out. The caller writes the simulation steps of a block before it renders the block.
- The runtime is not bit exact. Use the exact mode for the null test.
- The benchmarks ran on a shared machine. The phone figures in the answer are estimates from the factors of the earlier measurement.
