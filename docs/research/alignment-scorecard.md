# Alignment scorecard: snarl against the reference renders

Measured at `2547fc0` on the branch `upstream-profiles`: snarl tier 1 at 48 kHz, the profiles as committed, against the references in `out/reference/upstream/<engine>/` (engine-sim) and `out/reference/enginelab/f6-aircooled/` (EngineLab). The tool is `snarl-compare --align` (`cmd/snarl-compare/align.go`, `internal/analysis/align.go`).

## Answer

engine-sim against itself scores 100 on every calibration: 21 comparisons of the five engine-sim engines, with a second render or with the two halves of each hold. The current snarl scores 24 to 45 on the holds, 54 to 69 on the overrun and 73 to 88 on the full-throttle sweep. The sweep figure is an upper bound, because a sweep has no cycle measures and the sweep floors are loose. The target of 90 is far away on every engine.

| Engine | Holds t000 | Holds t050 | Holds t100 | Sweep | Overrun |
| --- | --- | --- | --- | --- | --- |
| `i4-vtec` | 24.3 | 31.2 | 36.8 | 73.0 | 53.6 |
| `v8-ls` | 23.7 | 31.8 | 31.8 | 76.3 | 54.6 |
| `f4-ej25` | 39.2 | 44.9 | 44.6 | 86.1 | 68.6 |
| `v12-412` | 38.5 | 34.8 | 39.8 | 88.1 | 68.1 |
| `v12-merlin` | 25.8 | 28.4 | 28.8 | 87.0 | 62.9 |
| `f6-aircooled` | no scenario | 19.4 | 20.2 | 37.1 | 25.5 |

`f6-aircooled` has only the spectral and roughness measures (section 3.3), and EngineLab against itself scores 79 to 86 on the same measures. Read its row as about a quarter of its own ceiling.

The differences that matter most for the listener, largest first (section 5):

1. The noise of snarl comes in a burst at each firing, where the noise of engine-sim is a steady bed. This is the hiss that the listener still hears, and the main part of "synthesised".
2. The shape of each engine cycle is wrong. The synchronous average cycle of snarl leaves an error only 1 to 12 dB below the signal. Two engine-sim renders leave 14 to 49 dB.
3. The short-time spectrum differs by 1 to 11 dB more than engine-sim differs from itself, most in 300 Hz to 2 kHz on `v12-merlin` and below 300 Hz on `v12-412`. `v12-412` shows sharp, fixed resonance lines that engine-sim does not have.
4. Each firing stands out of the 2 to 16 kHz bands by 18 to 60 dB in snarl, against 4 to 30 dB in engine-sim. The attack and decay of each firing do not match: the mean envelope correlation is 0.2 to 0.6 on `v8-ls`, against 0.8 to 1.0 for engine-sim against itself.
5. Two engine-specific faults: `i4-vtec` has a dropout and a 22 dB spectral error at the VTEC change near 5000 to 5300 rpm, and `f4-ej25` has no cycle-to-cycle rumble of the unequal headers.

## 1. How to run

```sh
# Render snarl, then score it against the reference.
go run ./cmd/snarl-render --out out/align/snarl/v8-ls/holds-t100.wav v8-ls scenarios/upstream/v8-ls/holds-t100.scn
go run ./cmd/snarl-compare --align --scenario scenarios/upstream/v8-ls/holds-t100.scn \
  out/reference/upstream/v8-ls/holds-t100.wav out/align/snarl/v8-ls/holds-t100.wav

# Self comparison: the first half of each hold against the second half.
go run ./cmd/snarl-compare --align --scenario scenarios/upstream/v8-ls/holds-t100.scn \
  out/reference/upstream/v8-ls/holds-t100.wav

# EngineLab reference: no cycle measures, and a longer settle time.
go run ./cmd/snarl-compare --align --cycles=false --settle 1 --scenario scenarios/upstream/f6-aircooled/holds-t100.scn \
  out/reference/enginelab/f6-aircooled/holds-t100.wav out/align/snarl/f6-aircooled/holds-t100.wav
```

A is the reference, B is the render. The command prints four tables: the scorecard of each window, the cycle detail of each hold, the noise of each hold, and the total with the mean steps of each measure and the worst value of each distance next to its floor. `--settle` (default 0.25 s) skips the start of each segment. `--window` (default 0.5 s) sets the length of the sweep windows.

All outputs of this study are in `out/align/`: the snarl renders in `snarl/`, the second engine-sim renders in `es-calib/`, the scorecards in `score/`, the calibrations in `calib/`, and the spectrograms in `spec/`.

## 2. Definitions

The windows are those of `analysis.Windows`: one per hold, after the settle time, and windows of about 0.5 s along each sweep. Each window gives the measures below. Sections 2.1 and 2.5 apply to every window. Sections 2.2 to 2.4 apply to holds only.

### 2.1 Frame-wise perceptual spectral distance

1. Both windows get the BS.1770 K-weighting. B is scaled so that its K-weighted mean square equals that of A. The scorecard reports the level difference before the scaling, less its mean over the windows, as the loudness contour.
2. Frames of 32 ms, Hann, overlapping by half, zero-padded to the next power of two of twice the frame.
3. Triangular filters on the ERB-rate scale (Glasberg and Moore, `21.4 log10(1 + 0.00437 f)`), one ERB apart, centres from 30 Hz to 16 kHz or 0.45 of the rate: 39 bands at 48 kHz.
4. Each band power gets a floor of 1e-6 times the strongest band of the frame in both files (-60 dB), then 10 log10.
5. The frame distance is the RMS over the bands of the dB difference. The scorecard reports the median and the 90th percentile over the frames, and the median in each band group: below 300 Hz, 300 Hz to 2 kHz, 2 to 6 kHz, above 6 kHz.
6. For a hold, B is shifted by the lag of the cycle waveform match (2.2), so that the frames see the same crank angle in both files.
7. The self distance is the same measure of A against A, shifted by the whole number of cycles that first reaches a frame length. It is the floor that the random part of A gives this window. The score counts the distance above it.

### 2.2 Cycle waveform match

1. The hold is resampled to the crank angle domain over whole 720-degree cycles with the residual search of `--residual`: the cycle period within 0.5 per cent of the nominal RPM that leaves the least residual, and a Kaiser-windowed sinc interpolator. When that period is at the edge of the search, the search runs again at RPM steps of 0.9 per cent, up to 4.5 per cent.
2. The cycles are resampled again to a power of two of samples per cycle, and averaged: the synchronous average cycle.
3. The circular cross-correlation of the two average cycles (FFT) gives the best shift. The best gain then leaves an error of `1 - corr^2` of the power of A. The scorecard reports `corr` and the error below the signal, `-10 log10(1 - corr^2)` dB.
4. The correlation at the same shift inside the bands below 300 Hz, 300 Hz to 2 kHz and 2 to 6 kHz comes from the harmonics of the cycle in each band.

### 2.3 Temporal envelope match

1. Inside the angle domain, each cycle splits into 96 sectors. A 2 ms Hann frame at the start of each sector gives the power in 300 Hz to 2 kHz, 2 to 6 kHz and 6 to 16 kHz.
2. The mean over the cycles, in dB, is the envelope of one firing cycle in each band: the attack and decay of each firing.
3. B is shifted by the waveform lag, give or take one sector. The scorecard reports the correlation of the two envelopes in each band, their mean, and the depth of each envelope (highest less lowest value).
4. The envelope distance is the mean over the bands of the RMS difference of the two envelopes, each less its mean. The score uses this distance and not the correlation, because the correlation of two flat envelopes is noise.

### 2.4 Noise character

The residual of `--residual` (the part that does not repeat from cycle to cycle, relative to the signal) and the burst measure of `--burst` (modulation depth Mod, cycle-locked part Locked, crest and flatness in 2 to 4, 4 to 8 and 8 to 16 kHz), both files. The residual distance is the difference of the two residual levels, each at least -35 dB. The burst distance is the mean absolute difference of Mod over the bands.

### 2.5 Roughness (all windows)

The sweeps have no steady cycle, so a measure with no cycle alignment covers their temporal envelope. In each envelope band, the dB envelope of 2 ms frames (hop 1 ms), less its 100 ms moving mean, gives the roughness: its RMS in dB. A steady noise gives 1 to 3 dB from the frame statistics. Firings that stand out of the band, or noise in bursts, give more. The roughness distance is the mean absolute difference over the bands.

### 2.6 Score

Each distance beyond its calibration floor counts in steps. Each step costs a tenth of the score:

```text
steps_i = max(0, d_i - floor_i) / step_i        (for the wave error: max(0, floor - e) / step)
score   = 100 * 0.9 ^ (sum of steps_i)
```

| Distance d_i | Floor | Step |
| --- | --- | --- |
| Spectral median above self, hold | 0.5 dB | 1 dB |
| Spectral p90 above self, hold | 1.0 dB | 2 dB |
| Spectral median above self, sweep | 4.0 dB | 1 dB |
| Spectral p90 above self, sweep | 5.5 dB | 2 dB |
| Roughness distance | 0.6 dB | 1 dB |
| Wave error below the signal (shortfall) | 14 dB | 4 dB |
| Envelope distance | 1.5 dB | 2 dB |
| Residual distance | 2.0 dB | 2 dB |
| Burst Mod distance | 1.3 dB | 2 dB |

A distance at its floor costs nothing, so engine-sim against itself scores 100. The score falls by a factor of 0.9 for each step and has no jump. A score of 90 is one step in total, for example a spectral median 1 dB above the floor. A measure that the window does not have counts zero steps. The total of a file is the mean of the window scores, weighted by the window length. The floors are the constants `AlignFloor*` and the steps `AlignStep*` in `internal/analysis/align.go`.

## 3. Calibration

### 3.1 Method

engine-sim-render has no seed option. Two comparisons count as "the same sound":

- A second render of the same scenario with a pre-roll of 3.5 s in place of 3 s (`--settle 3.5`, `out/align/es-calib/<engine>/`). The engine and the noise sources start from another state, and the scenario is the same. Pairs: `holds-t100`, `sweep-wot` and `overrun` of the five engine-sim engines.
- The first half of each hold against the second half of the same hold (one file to `--align`). Splits: `holds-t000` of `v8-ls`, `holds-t050` of all five engine-sim engines, and `holds-t050` and `holds-t100` of `f6-aircooled`.

### 3.2 engine-sim

All 21 engine-sim comparisons score 100.0. The worst value of each distance over them:

| Distance | Worst | Where | Floor |
| --- | --- | --- | --- |
| Spectral median above self, hold | 0.17 dB | `v12-412` pair | 0.5 dB |
| Spectral p90 above self, hold | 0.35 dB | `v12-merlin` split | 1.0 dB |
| Spectral median above self, sweep | 3.65 dB | `f4-ej25` sweep, 2690 to 3028 rpm | 4.0 dB |
| Spectral p90 above self, sweep | 5.26 dB | `f4-ej25` sweep | 5.5 dB |
| Roughness distance | 0.56 dB | `v12-merlin` pair | 0.6 dB |
| Wave error, least | 14.2 dB | `v12-merlin` split, 600 rpm (6 cycles a half) | 14 dB |
| Envelope distance | 1.31 dB | `v8-ls` split, `holds-t050` | 1.5 dB |
| Residual distance | 1.99 dB | `i4-vtec` split, `holds-t050` | 2.0 dB |
| Burst Mod distance | 1.28 dB | `v12-merlin` split, 600 rpm | 1.3 dB |

What the calibration shows:

- Two engine-sim renders of a hold are almost the same signal. The average cycles match to 23 to 49 dB below the signal, and the self distance of A predicts the pair distance to 0.4 dB. The frame-wise spectral distance of engine-sim against itself is 0.7 to 2.6 dB on the holds, all from the noise.
- Sweeps are the weak point. Two engine-sim renders of a sweep differ by up to 3.7 dB more than the self distance, because the two RPM paths differ a little and narrow resonances pass at other times. The sweep floors are therefore loose, and the spectral measure of a sweep can only see large errors.
- Splits are harder than pairs at idle, because each half holds only a few cycles. The waveform floor of 14 dB comes from the `v12-merlin` idle at 600 rpm. Every pair gives at least 23 dB.

### 3.3 EngineLab (`f6-aircooled`)

The EngineLab holds do not keep a steady RPM: the governor wanders by 0.5 to 1.2 per cent inside a hold (for example 4011 against 4044 rpm in the two halves of the 4000 rpm hold). The cycle average then fails: the residual of EngineLab against itself is -0.3 to -3.7 dB, and the split scores 47 with the cycle measures. With `--cycles=false`, the splits score 86 (`holds-t050`) and 79 (`holds-t100`), with spectral p90 up to 9.8 dB above self. For `f6-aircooled`, only the spectral and roughness measures count, and the ceiling is about 80.

## 4. Current snarl

### 4.1 Which measures fail most

Mean steps per window of `holds-t100` (the other holds give the same order):

| Measure | `i4-vtec` | `v8-ls` | `f4-ej25` | `v12-412` | `v12-merlin` | Mean |
| --- | --- | --- | --- | --- | --- | --- |
| Spectral (median and p90) | 5.81 | 1.60 | 2.24 | 5.64 | 5.16 | 4.09 |
| Cycle waveform | 1.70 | 1.98 | 1.20 | 2.40 | 2.41 | 1.94 |
| Envelope | 2.36 | 2.22 | 2.05 | 0.30 | 1.84 | 1.75 |
| Roughness | 2.43 | 2.51 | 1.85 | 0.40 | 1.48 | 1.73 |
| Residual | 2.50 | 1.50 | 1.44 | 1.00 | 0.00 | 1.29 |
| Burst | 0.49 | 1.61 | 0.32 | 0.19 | 1.55 | 0.83 |

On the sweeps, only roughness and the spectral measure count. Roughness gives 1.1 to 2.9 steps on `i4-vtec`, `v8-ls`, `f4-ej25` and `v12-merlin`. The spectral measure gives 0.9 steps on `v12-412` and 3.1 steps on `i4-vtec`. On `f6-aircooled`, the spectral median gives 8 to 12 steps and the p90 4 to 6 steps.

Measured values behind the steps, `holds-t100`, engine-sim against snarl:

- Cycle waveform correlation: `v8-ls` 0.48 to 0.97 (error 1.1 to 12.2 dB), against 1.000 for two engine-sim renders. Below 300 Hz, the `v8-ls` correlation is only 0.28 to 0.68 from 2000 to 5000 rpm. In 2 to 6 kHz it is -0.27 to 0.42 on every engine.
- Envelope depth in 2 to 6 kHz: `v8-ls` 3.8 to 27.3 dB against 18.4 to 47.4 dB. `i4-vtec` at 5000 rpm: 9.3 dB against 52.8 dB. In 6 to 16 kHz, `v8-ls` 5.5 to 30.4 dB against 22.2 to 60.4 dB.
- Locked part of the residual modulation, 2 to 4 kHz: on `v8-ls`, engine-sim -0.1 to -17.8 dB and snarl +4.9 to -3.2 dB. On `v12-merlin`, -4.9 to -9.0 dB against -1.0 to +0.8 dB.
- Residual level: snarl has 2 to 10 dB more residual than engine-sim at 2000 rpm and up on `i4-vtec`, `v8-ls` and `f4-ej25` (for example `v8-ls` at 6500 rpm, -31.5 against -21.8 dB).
- Spectral band groups: `v12-merlin` 300 Hz to 2 kHz is 9.4 to 10.6 dB at 2000 and 3000 rpm, against 2 to 3 dB for engine-sim against itself. `v12-412` below 300 Hz is 5 to 8 dB. `i4-vtec` at 5000 rpm has a 22.5 dB median, with snarl 9.8 dB quieter than its contour and a residual of -10.7 dB.

### 4.2 Spectrograms

`out/align/spec/<engine>-sweep.png` stacks the `sweep-wot` spectrograms (engine-sim above, snarl below). `out/align/spec/<engine>-hold3000.png` (`v12-412`: `hold6000.png`) stacks 0.25 s of the 3000 rpm full-throttle hold. Both are log frequency and log magnitude, at the absolute level of each file.

- `i4-vtec`: engine-sim is smooth from 20 Hz to 8 kHz, with one step up in the mids at the VTEC change near 5.3 s. snarl has a broken region from 4.3 to 5.3 s (the low band drops and a comb pattern fills 400 Hz to 3 kHz), then a silent gap of a few milliseconds at about 5.35 s. In the hold, snarl adds a strong fixed line near 350 Hz, and its noise above 1 kHz comes in a stripe at each firing.
- `v8-ls`: the sweeps look alike at this scale. In the 3000 rpm hold, the engine-sim noise above 1 kHz is an even texture, and the snarl noise is cut into vertical stripes, one per firing, with dark gaps between them.
- `f4-ej25`: engine-sim shows a pattern that repeats every engine cycle (about 40 ms at 3000 rpm), with bright and dark firings from the unequal headers, and noise bursts up to 10 kHz. snarl has even firings with no cycle-to-cycle pattern. The sweep shapes are close.
- `v12-412`: engine-sim is broadband noise to 10 kHz with blurred harmonics. snarl shows two or three sharp lines that rise with the RPM in the sweep and stay fixed near 600 Hz, 900 Hz and 1.2 kHz in the hold, with less noise between them. This is the clean, synthesised look.
- `v12-merlin`: engine-sim has bright speckled noise from 2 to 8 kHz in the first 3 s of the sweep. snarl is smoother and darker there. The holds look alike at this scale.
- `f6-aircooled`: EngineLab is quieter in absolute level, has deep notches near 3.5 kHz and 10 kHz, and a chirp in 200 to 500 Hz at each firing. snarl has a strong, steady band below 300 Hz and no notches.

## 5. The differences that matter most for the listener

Ranked by how much of the score they cost and by how plainly they show in the spectrograms.

1. **Noise in bursts at each firing.** Envelope (1.75 steps), roughness (1.73) and burst (0.83) together cost more than any other group on the holds. In snarl, each firing brings a burst of noise from 1 to 16 kHz, with silence between the bursts: the firing stands 18 to 60 dB out of the 2 to 16 kHz bands, and the noise is locked to the cycle (Locked up to +4.9 dB). engine-sim has a steady noise bed that the firings modulate by only 4 to 30 dB, mostly unlocked. A gated noise like this sounds like a synthesiser hiss on top of the engine, not part of it. This matches "the hiss is still there" and "synthesised".
2. **Wrong cycle waveform.** The average cycle of snarl matches engine-sim to only 1 to 12 dB, against 14 to 49 dB for engine-sim against itself, and below 300 Hz the correlation is 0.3 to 0.7 at mid RPM on `v8-ls`. The low-frequency pulse shape of each firing is what gives an engine its character, so this matches "characterless". A port of the engine-sim synthesiser fixes this only if its input pressure pulses match too.
3. **Spectral fine structure.** The frame-wise spectrum of snarl is 1 to 11 dB further from engine-sim than engine-sim is from itself: `v12-merlin` in 300 Hz to 2 kHz, `v12-412` below 300 Hz with fixed resonance lines, `f6-aircooled` with far more bass and no notches. Fixed narrow lines over a clean background sound synthetic.
4. **More noise at mid and high RPM.** The residual of snarl is 2 to 10 dB higher than that of engine-sim on `i4-vtec`, `v8-ls` and `f4-ej25`. With item 1, this is a louder hiss that pulses.
5. **Engine-specific faults.** `i4-vtec`: a broken region and a dropout at the VTEC change (22.5 dB spectral median at the 5000 rpm hold). `f4-ej25`: no cycle-to-cycle pattern from the unequal headers, which is the boxer rumble. Both are clear to a listener on those engines.

What the scorecard does not cover:

- Sweeps have no cycle measures, so the sweep scores of 73 to 88 hide the burst noise and the waveform error that the holds show. Read the hold scores as the main figure.
- The `f6-aircooled` score uses the spectral and roughness measures only, because the EngineLab holds do not keep a steady RPM.
- Two engine-sim renders of a sweep differ by up to 3.7 dB in the frame-wise spectrum, so a sweep error below that does not show.
